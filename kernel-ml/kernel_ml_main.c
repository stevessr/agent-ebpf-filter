/* Kernel ML Module - Main Entry Point */

#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/init.h>
#include <linux/jiffies.h>
#include <linux/kobject.h>
#include <linux/proc_fs.h>
#include <linux/poll.h>
#include <linux/seq_file.h>
#include <linux/slab.h>
#include <linux/string.h>
#include <linux/sysfs.h>
#include <linux/uaccess.h>
#include <linux/wait.h>
#include "ml_inference.h"

#define MODULE_NAME "kernel_ml"
#define PROC_MODEL_LOAD "ml_load"
#define PROC_MODEL_PREDICT "ml_predict"
#define PROC_MODEL_STATS "ml_stats"
#define PROC_MODEL_BACKEND "ml_backend"
#define PROC_CUDA_REQUEST "ml_cuda_request"
#define PROC_CUDA_RESULT "ml_cuda_result"
#define PROC_CUDA_MODEL "ml_cuda_model"
#define ML_CACHE_SIZE 64

static struct ml_model global_model;
static atomic64_t inference_count = ATOMIC64_INIT(0);
static atomic64_t block_count = ATOMIC64_INIT(0);
static atomic64_t alert_count = ATOMIC64_INIT(0);
static atomic64_t cuda_inference_count = ATOMIC64_INIT(0);
static atomic64_t cuda_fallback_count = ATOMIC64_INIT(0);
static atomic64_t cuda_timeout_count = ATOMIC64_INIT(0);
static atomic64_t cuda_busy_count = ATOMIC64_INIT(0);
static atomic64_t cuda_error_count = ATOMIC64_INIT(0);
static atomic64_t model_generation = ATOMIC64_INIT(0);

static char *backend_param = "kernel";
static unsigned int cuda_timeout_ms = ML_CUDA_DEFAULT_TIMEOUT_MS;
static bool cache_enabled = true;
module_param_named(backend, backend_param, charp, 0644);
MODULE_PARM_DESC(backend, "Inference backend: kernel, cuda, or auto");
module_param(cuda_timeout_ms, uint, 0644);
MODULE_PARM_DESC(cuda_timeout_ms, "CUDA offload timeout in milliseconds");
module_param(cache_enabled, bool, 0644);
MODULE_PARM_DESC(cache_enabled, "Enable exact-match LRU inference cache");

static enum ml_backend_type active_backend = ML_BACKEND_KERNEL;

static DEFINE_MUTEX(model_blob_lock);
static DEFINE_MUTEX(model_lock);
static u8 *model_blob;
static size_t model_blob_size;

static DEFINE_MUTEX(cuda_lock);
static DECLARE_WAIT_QUEUE_HEAD(cuda_request_wq);
static DECLARE_WAIT_QUEUE_HEAD(cuda_result_wq);
static atomic64_t cuda_request_seq = ATOMIC64_INIT(0);
static atomic_t cuda_clients = ATOMIC_INIT(0);
static bool cuda_inflight;
static bool cuda_request_pending;
static bool cuda_result_ready;
static bool cuda_shutting_down;
static struct ml_cuda_request cuda_request;
static struct ml_cuda_result cuda_result;

struct ml_cache_entry {
	bool valid;
	u64 generation;
	u64 last_used;
	enum ml_action action;
	struct feature_vector fv;
};

static DEFINE_MUTEX(cache_lock);
static struct ml_cache_entry inference_cache[ML_CACHE_SIZE];
static u64 cache_clock;
static atomic64_t cache_hit_count = ATOMIC64_INIT(0);
static atomic64_t cache_miss_count = ATOMIC64_INIT(0);
static atomic64_t cache_store_count = ATOMIC64_INIT(0);

static struct kobject *kernel_ml_kobj;

static const char *ml_backend_name(enum ml_backend_type backend)
{
	switch (backend) {
	case ML_BACKEND_KERNEL:
		return "kernel";
	case ML_BACKEND_CUDA:
		return "cuda";
	case ML_BACKEND_AUTO:
		return "auto";
	default:
		return "unknown";
	}
}

static const char *ml_action_label(enum ml_action action, char *buf, size_t len)
{
	switch (action) {
	case ML_ACTION_ALLOW:
		return "ALLOW";
	case ML_ACTION_BLOCK:
		return "BLOCK";
	case ML_ACTION_ALERT:
		return "ALERT";
	default:
		scnprintf(buf, len, "CLASS_%u", (u32)action);
		return buf;
	}
}

static int ml_backend_from_string(char *raw, enum ml_backend_type *backend)
{
	char *name = strim(raw);

	if (sysfs_streq(name, "kernel") || sysfs_streq(name, "cpu")) {
		*backend = ML_BACKEND_KERNEL;
		return 0;
	}
	if (sysfs_streq(name, "cuda") || sysfs_streq(name, "gpu")) {
		*backend = ML_BACKEND_CUDA;
		return 0;
	}
	if (sysfs_streq(name, "auto")) {
		*backend = ML_BACKEND_AUTO;
		return 0;
	}
	return -EINVAL;
}

static void ml_store_model_blob(void *raw, size_t size)
{
	u8 *old_blob;

	mutex_lock(&model_blob_lock);
	old_blob = model_blob;
	model_blob = raw;
	model_blob_size = size;
	atomic64_inc(&model_generation);
	mutex_unlock(&model_blob_lock);

	kfree(old_blob);
}

static void ml_cache_reset(void)
{
	mutex_lock(&cache_lock);
	memset(inference_cache, 0, sizeof(inference_cache));
	cache_clock = 0;
	mutex_unlock(&cache_lock);
}

static bool ml_cache_lookup(const struct feature_vector *fv, enum ml_action *action)
{
	u64 generation;
	int i;

	if (!cache_enabled || !fv || !action)
		return false;

	generation = (u64)atomic64_read(&model_generation);
	mutex_lock(&cache_lock);
	for (i = 0; i < ML_CACHE_SIZE; i++) {
		if (!inference_cache[i].valid ||
		    inference_cache[i].generation != generation)
			continue;
		if (memcmp(&inference_cache[i].fv, fv, sizeof(*fv)) != 0)
			continue;
		inference_cache[i].last_used = ++cache_clock;
		*action = inference_cache[i].action;
		mutex_unlock(&cache_lock);
		atomic64_inc(&cache_hit_count);
		return true;
	}
	mutex_unlock(&cache_lock);
	atomic64_inc(&cache_miss_count);
	return false;
}

static void ml_cache_store(const struct feature_vector *fv, enum ml_action action)
{
	u64 generation;
	int i;
	int victim = 0;

	if (!cache_enabled || !fv)
		return;

	generation = (u64)atomic64_read(&model_generation);
	mutex_lock(&cache_lock);
	for (i = 0; i < ML_CACHE_SIZE; i++) {
		if (!inference_cache[i].valid) {
			victim = i;
			goto store;
		}
		if (inference_cache[i].last_used < inference_cache[victim].last_used)
			victim = i;
	}

store:
	inference_cache[victim].valid = true;
	inference_cache[victim].generation = generation;
	inference_cache[victim].last_used = ++cache_clock;
	inference_cache[victim].action = action;
	inference_cache[victim].fv = *fv;
	mutex_unlock(&cache_lock);
	atomic64_inc(&cache_store_count);
}

static void ml_reset_cuda_request_locked(void)
{
	memset(&cuda_request, 0, sizeof(cuda_request));
	memset(&cuda_result, 0, sizeof(cuda_result));
	cuda_inflight = false;
	cuda_request_pending = false;
	cuda_result_ready = false;
}

static int ml_cuda_inference(struct feature_vector *fv, enum ml_action *action)
{
	u64 request_id;
	long timeout;
	unsigned int wait_ms;
	int ret = 0;

	if (!fv || !action)
		return -EINVAL;
	if (cuda_shutting_down)
		return -ESHUTDOWN;
	if (atomic_read(&cuda_clients) <= 0)
		return -ENODEV;

	mutex_lock(&cuda_lock);
	if (cuda_inflight) {
		mutex_unlock(&cuda_lock);
		atomic64_inc(&cuda_busy_count);
		return -EBUSY;
	}

	request_id = (u64)atomic64_inc_return(&cuda_request_seq);
	memset(&cuda_request, 0, sizeof(cuda_request));
	memset(&cuda_result, 0, sizeof(cuda_result));
	cuda_request.version = ML_CUDA_REQUEST_VERSION;
	cuda_request.request_id = request_id;
	cuda_request.model_generation = (u64)atomic64_read(&model_generation);
	cuda_request.features = *fv;
	cuda_inflight = true;
	cuda_request_pending = true;
	cuda_result_ready = false;
	mutex_unlock(&cuda_lock);

	wake_up_interruptible(&cuda_request_wq);

	wait_ms = cuda_timeout_ms ? cuda_timeout_ms : ML_CUDA_DEFAULT_TIMEOUT_MS;
	timeout = wait_event_timeout(cuda_result_wq,
	                             cuda_result_ready &&
	                                     cuda_result.request_id == request_id,
	                             msecs_to_jiffies(wait_ms));
	if (timeout == 0) {
		mutex_lock(&cuda_lock);
		if (cuda_inflight && cuda_request.request_id == request_id)
			ml_reset_cuda_request_locked();
		mutex_unlock(&cuda_lock);
		atomic64_inc(&cuda_timeout_count);
		wake_up_interruptible(&cuda_request_wq);
		return -ETIMEDOUT;
	}

	mutex_lock(&cuda_lock);
	if (!cuda_result_ready || cuda_result.request_id != request_id) {
		ret = -EIO;
	} else if (cuda_result.status != 0) {
		ret = -EREMOTEIO;
	} else if (cuda_result.action >=
	           (global_model.num_classes ? global_model.num_classes : ML_DEFAULT_NUM_CLASSES)) {
		ret = -EINVAL;
	} else {
		*action = (enum ml_action)cuda_result.action;
	}
	ml_reset_cuda_request_locked();
	mutex_unlock(&cuda_lock);
	wake_up_interruptible(&cuda_request_wq);

	if (ret)
		atomic64_inc(&cuda_error_count);
	else
		atomic64_inc(&cuda_inference_count);

	return ret;
}

static enum ml_action ml_run_inference(struct ml_model *model, struct feature_vector *fv)
{
	enum ml_action action;
	int ret;

	if (ml_cache_lookup(fv, &action))
		return action;

	if (active_backend == ML_BACKEND_KERNEL) {
		action = ml_inference(model, fv);
		ml_cache_store(fv, action);
		return action;
	}

	if (active_backend == ML_BACKEND_AUTO && atomic_read(&cuda_clients) <= 0) {
		action = ml_inference(model, fv);
		ml_cache_store(fv, action);
		return action;
	}

	ret = ml_cuda_inference(fv, &action);
	if (!ret) {
		ml_cache_store(fv, action);
		return action;
	}

	atomic64_inc(&cuda_fallback_count);
	action = ml_inference(model, fv);
	ml_cache_store(fv, action);
	return action;
}

/* Proc: Load model from userspace */
static ssize_t proc_model_load_write(struct file *file, const char __user *buffer,
                                     size_t count, loff_t *pos)
{
	void *raw;
	struct ml_model new_model;
	int ret;

	if (count > 10 * 1024 * 1024)
		return -EINVAL;

	raw = memdup_user(buffer, count);
	if (IS_ERR(raw))
		return PTR_ERR(raw);

	memset(&new_model, 0, sizeof(new_model));
	ret = ml_model_load(&new_model, buffer, count);
	if (ret) {
		kfree(raw);
		ml_model_free(&new_model);
		return ret;
	}

	mutex_lock(&model_lock);
	ml_model_free(&global_model);
	global_model = new_model;
	ml_store_model_blob(raw, count);
	ml_cache_reset();
	mutex_unlock(&model_lock);
	return count;
}

/* Proc: Perform inference */
static ssize_t proc_model_predict_write(struct file *file, const char __user *buffer,
                                        size_t count, loff_t *pos)
{
	struct feature_vector fv;
	enum ml_action action;
	char action_buf[24];

	if (count != sizeof(fv))
		return -EINVAL;

	if (copy_from_user(&fv, buffer, sizeof(fv)))
		return -EFAULT;

	mutex_lock(&model_lock);
	action = ml_run_inference(&global_model, &fv);
	mutex_unlock(&model_lock);
	atomic64_inc(&inference_count);

	switch (action) {
	case ML_ACTION_BLOCK:
		atomic64_inc(&block_count);
		break;
	case ML_ACTION_ALERT:
		atomic64_inc(&alert_count);
		break;
	default:
		break;
	}

	pr_info("kernel-ml: PID %u (%s) syscall %u -> %s",
	        fv.pid, fv.comm, fv.syscall_type,
	        ml_action_label(action, action_buf, sizeof(action_buf)));

	return count;
}

/* Proc: Stats */
static int proc_model_stats_show(struct seq_file *m, void *v)
{
	seq_printf(m, "Model Version: %u\n", global_model.version);
	seq_printf(m, "Trees: %u\n", global_model.num_trees);
	seq_printf(m, "Features: %u\n", global_model.feature_dim);
	seq_printf(m, "Classes: %u\n", global_model.num_classes);
	seq_printf(m, "Max Depth: %u\n", global_model.max_depth);
	seq_printf(m, "Model Generation: %llu\n",
	           (unsigned long long)atomic64_read(&model_generation));
	seq_printf(m, "Model Blob Bytes: %zu\n", model_blob_size);
	seq_printf(m, "Backend: %s\n", ml_backend_name(active_backend));
	seq_printf(m, "CUDA Timeout Ms: %u\n", cuda_timeout_ms);
	seq_printf(m, "CUDA Helper Clients: %d\n", atomic_read(&cuda_clients));
	seq_printf(m, "Total Inferences: %llu\n",
	           (unsigned long long)atomic64_read(&inference_count));
	seq_printf(m, "CUDA Inferences: %llu\n",
	           (unsigned long long)atomic64_read(&cuda_inference_count));
	seq_printf(m, "CUDA Fallbacks: %llu\n",
	           (unsigned long long)atomic64_read(&cuda_fallback_count));
	seq_printf(m, "CUDA Timeouts: %llu\n",
	           (unsigned long long)atomic64_read(&cuda_timeout_count));
	seq_printf(m, "CUDA Busy: %llu\n",
	           (unsigned long long)atomic64_read(&cuda_busy_count));
	seq_printf(m, "CUDA Errors: %llu\n",
	           (unsigned long long)atomic64_read(&cuda_error_count));
	seq_printf(m, "Cache Enabled: %u\n", cache_enabled ? 1 : 0);
	seq_printf(m, "Cache Entries: %u\n", ML_CACHE_SIZE);
	seq_printf(m, "Cache Hits: %llu\n",
	           (unsigned long long)atomic64_read(&cache_hit_count));
	seq_printf(m, "Cache Misses: %llu\n",
	           (unsigned long long)atomic64_read(&cache_miss_count));
	seq_printf(m, "Cache Stores: %llu\n",
	           (unsigned long long)atomic64_read(&cache_store_count));
	seq_printf(m, "Blocks: %llu\n", (unsigned long long)atomic64_read(&block_count));
	seq_printf(m, "Alerts: %llu\n", (unsigned long long)atomic64_read(&alert_count));
	return 0;
}

static int proc_stats_open(struct inode *inode, struct file *file)
{
	return single_open(file, proc_model_stats_show, NULL);
}

static int proc_backend_show(struct seq_file *m, void *v)
{
	seq_printf(m, "backend=%s\n", ml_backend_name(active_backend));
	seq_printf(m, "cuda_timeout_ms=%u\n", cuda_timeout_ms);
	seq_printf(m, "cuda_helper_clients=%d\n", atomic_read(&cuda_clients));
	seq_printf(m, "usage: echo kernel|cuda|auto > /proc/%s\n", PROC_MODEL_BACKEND);
	seq_printf(m, "usage: echo timeout_ms=<1..60000> > /proc/%s\n", PROC_MODEL_BACKEND);
	return 0;
}

static int proc_backend_open(struct inode *inode, struct file *file)
{
	return single_open(file, proc_backend_show, NULL);
}

static ssize_t proc_backend_write(struct file *file, const char __user *buffer,
                                  size_t count, loff_t *pos)
{
	char cmd[64];
	char *value;
	size_t n;
	enum ml_backend_type backend;
	unsigned int timeout_value;
	int ret;

	n = min(count, sizeof(cmd) - 1);
	if (copy_from_user(cmd, buffer, n))
		return -EFAULT;
	cmd[n] = '\0';
	value = strim(cmd);

	if (str_has_prefix(value, "timeout_ms=")) {
		ret = kstrtouint(value + strlen("timeout_ms="), 10, &timeout_value);
		if (ret)
			return ret;
		if (timeout_value == 0 || timeout_value > 60000)
			return -EINVAL;
		cuda_timeout_ms = timeout_value;
		pr_info("kernel-ml: CUDA timeout set to %u ms\n", cuda_timeout_ms);
		return count;
	}

	ret = ml_backend_from_string(value, &backend);
	if (ret)
		return ret;

	active_backend = backend;
	pr_info("kernel-ml: inference backend set to %s\n", ml_backend_name(active_backend));
	return count;
}

static int proc_cuda_request_open(struct inode *inode, struct file *file)
{
	atomic_inc(&cuda_clients);
	return 0;
}

static int proc_cuda_request_release(struct inode *inode, struct file *file)
{
	if (atomic_dec_return(&cuda_clients) < 0)
		atomic_set(&cuda_clients, 0);
	return 0;
}

static ssize_t proc_cuda_request_read(struct file *file, char __user *buffer,
                                      size_t count, loff_t *pos)
{
	struct ml_cuda_request req;
	int ret;

	if (count < sizeof(req))
		return -EINVAL;

	if (!cuda_request_pending) {
		if (file->f_flags & O_NONBLOCK)
			return -EAGAIN;
		ret = wait_event_interruptible(cuda_request_wq,
		                               cuda_request_pending || cuda_shutting_down);
		if (ret)
			return ret;
	}

	mutex_lock(&cuda_lock);
	if (cuda_shutting_down) {
		mutex_unlock(&cuda_lock);
		return -ESHUTDOWN;
	}
	if (!cuda_request_pending) {
		mutex_unlock(&cuda_lock);
		return -EAGAIN;
	}
	req = cuda_request;
	cuda_request_pending = false;
	mutex_unlock(&cuda_lock);

	if (copy_to_user(buffer, &req, sizeof(req)))
		return -EFAULT;

	return sizeof(req);
}

static __poll_t proc_cuda_request_poll(struct file *file, poll_table *wait)
{
	__poll_t mask = 0;

	poll_wait(file, &cuda_request_wq, wait);
	if (cuda_request_pending)
		mask |= POLLIN | POLLRDNORM;
	if (cuda_shutting_down)
		mask |= POLLHUP;
	return mask;
}

static ssize_t proc_cuda_result_write(struct file *file, const char __user *buffer,
                                      size_t count, loff_t *pos)
{
	struct ml_cuda_result result;
	int ret = 0;

	if (count != sizeof(result))
		return -EINVAL;
	if (copy_from_user(&result, buffer, sizeof(result)))
		return -EFAULT;
	if (result.version != ML_CUDA_REQUEST_VERSION)
		return -EINVAL;
	if (result.status == 0 && result.action >=
	    (global_model.num_classes ? global_model.num_classes : ML_DEFAULT_NUM_CLASSES))
		return -EINVAL;

	mutex_lock(&cuda_lock);
	if (!cuda_inflight || result.request_id != cuda_request.request_id) {
		ret = -ESTALE;
	} else {
		cuda_result = result;
		cuda_result_ready = true;
	}
	mutex_unlock(&cuda_lock);

	if (ret) {
		atomic64_inc(&cuda_error_count);
		return ret;
	}

	wake_up(&cuda_result_wq);
	return count;
}

static ssize_t proc_cuda_model_read(struct file *file, char __user *buffer,
                                    size_t count, loff_t *pos)
{
	ssize_t ret;
	size_t remaining;
	size_t n;

	mutex_lock(&model_blob_lock);
	if (!model_blob || *pos >= model_blob_size) {
		mutex_unlock(&model_blob_lock);
		return 0;
	}
	remaining = model_blob_size - *pos;
	n = min(count, remaining);
	if (copy_to_user(buffer, model_blob + *pos, n)) {
		ret = -EFAULT;
	} else {
		*pos += n;
		ret = n;
	}
	mutex_unlock(&model_blob_lock);

	return ret;
}

static ssize_t sysfs_backend_show(struct kobject *kobj,
                                  struct kobj_attribute *attr, char *buf)
{
	return sysfs_emit(buf, "%s\n", ml_backend_name(active_backend));
}

static ssize_t sysfs_backend_store(struct kobject *kobj,
                                   struct kobj_attribute *attr,
                                   const char *buf, size_t count)
{
	char tmp[32];
	enum ml_backend_type backend;

	if (count >= sizeof(tmp))
		return -EINVAL;
	memcpy(tmp, buf, count);
	tmp[count] = '\0';
	if (ml_backend_from_string(tmp, &backend))
		return -EINVAL;
	active_backend = backend;
	return count;
}

static ssize_t sysfs_cuda_timeout_show(struct kobject *kobj,
                                       struct kobj_attribute *attr, char *buf)
{
	return sysfs_emit(buf, "%u\n", cuda_timeout_ms);
}

static ssize_t sysfs_cuda_timeout_store(struct kobject *kobj,
                                        struct kobj_attribute *attr,
                                        const char *buf, size_t count)
{
	unsigned int value;

	if (kstrtouint(buf, 10, &value))
		return -EINVAL;
	if (value == 0 || value > 60000)
		return -EINVAL;
	cuda_timeout_ms = value;
	return count;
}

static ssize_t sysfs_cache_enabled_show(struct kobject *kobj,
                                        struct kobj_attribute *attr, char *buf)
{
	return sysfs_emit(buf, "%u\n", cache_enabled ? 1 : 0);
}

static ssize_t sysfs_cache_enabled_store(struct kobject *kobj,
                                         struct kobj_attribute *attr,
                                         const char *buf, size_t count)
{
	bool value;

	if (kstrtobool(buf, &value))
		return -EINVAL;
	cache_enabled = value;
	if (!cache_enabled)
		ml_cache_reset();
	return count;
}

static ssize_t sysfs_model_info_show(struct kobject *kobj,
                                     struct kobj_attribute *attr, char *buf)
{
	return sysfs_emit(buf,
	                  "version=%u\n"
	                  "generation=%llu\n"
	                  "trees=%u\n"
	                  "features=%u\n"
	                  "classes=%u\n"
	                  "max_depth=%u\n"
	                  "blob_bytes=%zu\n",
	                  global_model.version,
	                  (unsigned long long)atomic64_read(&model_generation),
	                  global_model.num_trees,
	                  global_model.feature_dim,
	                  global_model.num_classes,
	                  global_model.max_depth,
	                  model_blob_size);
}

static ssize_t sysfs_cache_stats_show(struct kobject *kobj,
                                      struct kobj_attribute *attr, char *buf)
{
	return sysfs_emit(buf,
	                  "enabled=%u\n"
	                  "entries=%u\n"
	                  "hits=%llu\n"
	                  "misses=%llu\n"
	                  "stores=%llu\n",
	                  cache_enabled ? 1 : 0,
	                  ML_CACHE_SIZE,
	                  (unsigned long long)atomic64_read(&cache_hit_count),
	                  (unsigned long long)atomic64_read(&cache_miss_count),
	                  (unsigned long long)atomic64_read(&cache_store_count));
}

static ssize_t sysfs_stats_show(struct kobject *kobj,
                                struct kobj_attribute *attr, char *buf)
{
	return sysfs_emit(buf,
	                  "backend=%s\n"
	                  "total_inferences=%llu\n"
	                  "blocks=%llu\n"
	                  "alerts=%llu\n"
	                  "cuda_inferences=%llu\n"
	                  "cuda_fallbacks=%llu\n"
	                  "cuda_timeouts=%llu\n"
	                  "cuda_busy=%llu\n"
	                  "cuda_errors=%llu\n",
	                  ml_backend_name(active_backend),
	                  (unsigned long long)atomic64_read(&inference_count),
	                  (unsigned long long)atomic64_read(&block_count),
	                  (unsigned long long)atomic64_read(&alert_count),
	                  (unsigned long long)atomic64_read(&cuda_inference_count),
	                  (unsigned long long)atomic64_read(&cuda_fallback_count),
	                  (unsigned long long)atomic64_read(&cuda_timeout_count),
	                  (unsigned long long)atomic64_read(&cuda_busy_count),
	                  (unsigned long long)atomic64_read(&cuda_error_count));
}

static struct kobj_attribute sysfs_backend_attr =
	__ATTR(backend, 0600, sysfs_backend_show, sysfs_backend_store);
static struct kobj_attribute sysfs_cuda_timeout_attr =
	__ATTR(cuda_timeout_ms, 0600, sysfs_cuda_timeout_show, sysfs_cuda_timeout_store);
static struct kobj_attribute sysfs_cache_enabled_attr =
	__ATTR(cache_enabled, 0600, sysfs_cache_enabled_show, sysfs_cache_enabled_store);
static struct kobj_attribute sysfs_model_info_attr =
	__ATTR(model_info, 0400, sysfs_model_info_show, NULL);
static struct kobj_attribute sysfs_cache_stats_attr =
	__ATTR(cache_stats, 0400, sysfs_cache_stats_show, NULL);
static struct kobj_attribute sysfs_stats_attr =
	__ATTR(stats, 0400, sysfs_stats_show, NULL);

static struct attribute *kernel_ml_attrs[] = {
	&sysfs_backend_attr.attr,
	&sysfs_cuda_timeout_attr.attr,
	&sysfs_cache_enabled_attr.attr,
	&sysfs_model_info_attr.attr,
	&sysfs_cache_stats_attr.attr,
	&sysfs_stats_attr.attr,
	NULL,
};

static const struct attribute_group kernel_ml_attr_group = {
	.attrs = kernel_ml_attrs,
};

static const struct proc_ops proc_load_ops = {
	.proc_write = proc_model_load_write,
};

static const struct proc_ops proc_predict_ops = {
	.proc_write = proc_model_predict_write,
};

static const struct proc_ops proc_stats_ops = {
	.proc_open = proc_stats_open,
	.proc_read = seq_read,
	.proc_lseek = seq_lseek,
	.proc_release = single_release,
};

static const struct proc_ops proc_backend_ops = {
	.proc_open = proc_backend_open,
	.proc_read = seq_read,
	.proc_write = proc_backend_write,
	.proc_lseek = seq_lseek,
	.proc_release = single_release,
};

static const struct proc_ops proc_cuda_request_ops = {
	.proc_open = proc_cuda_request_open,
	.proc_read = proc_cuda_request_read,
	.proc_poll = proc_cuda_request_poll,
	.proc_release = proc_cuda_request_release,
};

static const struct proc_ops proc_cuda_result_ops = {
	.proc_write = proc_cuda_result_write,
};

static const struct proc_ops proc_cuda_model_ops = {
	.proc_read = proc_cuda_model_read,
};

static int __init kernel_ml_init(void)
{
	struct proc_dir_entry *entry;
	enum ml_backend_type requested_backend;

	memset(&global_model, 0, sizeof(global_model));
	cuda_shutting_down = false;
	if (!ml_backend_from_string(backend_param, &requested_backend)) {
		active_backend = requested_backend;
	} else {
		pr_warn("kernel-ml: invalid backend '%s', using kernel\n", backend_param);
		active_backend = ML_BACKEND_KERNEL;
	}

	entry = proc_create(PROC_MODEL_LOAD, 0200, NULL, &proc_load_ops);
	if (!entry)
		goto err_load;

	entry = proc_create(PROC_MODEL_PREDICT, 0200, NULL, &proc_predict_ops);
	if (!entry)
		goto err_predict;

	entry = proc_create(PROC_MODEL_STATS, 0444, NULL, &proc_stats_ops);
	if (!entry)
		goto err_stats;

	entry = proc_create(PROC_MODEL_BACKEND, 0600, NULL, &proc_backend_ops);
	if (!entry)
		goto err_backend;

	entry = proc_create(PROC_CUDA_REQUEST, 0400, NULL, &proc_cuda_request_ops);
	if (!entry)
		goto err_cuda_request;

	entry = proc_create(PROC_CUDA_RESULT, 0200, NULL, &proc_cuda_result_ops);
	if (!entry)
		goto err_cuda_result;

	entry = proc_create(PROC_CUDA_MODEL, 0400, NULL, &proc_cuda_model_ops);
	if (!entry)
		goto err_cuda_model;

	kernel_ml_kobj = kobject_create_and_add("kernel_ml", kernel_kobj);
	if (!kernel_ml_kobj)
		goto err_sysfs_kobj;
	if (sysfs_create_group(kernel_ml_kobj, &kernel_ml_attr_group))
		goto err_sysfs_group;

	pr_info("kernel-ml: Loaded. Interface: /proc/%s, /proc/%s, /proc/%s, backend=%s\n",
	        PROC_MODEL_LOAD, PROC_MODEL_PREDICT, PROC_MODEL_STATS,
	        ml_backend_name(active_backend));

	return 0;

err_sysfs_group:
	kobject_put(kernel_ml_kobj);
	kernel_ml_kobj = NULL;
err_sysfs_kobj:
	remove_proc_entry(PROC_CUDA_MODEL, NULL);
err_cuda_model:
	remove_proc_entry(PROC_CUDA_RESULT, NULL);
err_cuda_result:
	remove_proc_entry(PROC_CUDA_REQUEST, NULL);
err_cuda_request:
	remove_proc_entry(PROC_MODEL_BACKEND, NULL);
err_backend:
	remove_proc_entry(PROC_MODEL_STATS, NULL);
err_stats:
	remove_proc_entry(PROC_MODEL_PREDICT, NULL);
err_predict:
	remove_proc_entry(PROC_MODEL_LOAD, NULL);
err_load:
	return -ENOMEM;
}

static void __exit kernel_ml_exit(void)
{
	u8 *old_blob;

	if (kernel_ml_kobj) {
		sysfs_remove_group(kernel_ml_kobj, &kernel_ml_attr_group);
		kobject_put(kernel_ml_kobj);
		kernel_ml_kobj = NULL;
	}
	remove_proc_entry(PROC_CUDA_MODEL, NULL);
	remove_proc_entry(PROC_CUDA_RESULT, NULL);
	remove_proc_entry(PROC_CUDA_REQUEST, NULL);
	remove_proc_entry(PROC_MODEL_BACKEND, NULL);
	remove_proc_entry(PROC_MODEL_STATS, NULL);
	remove_proc_entry(PROC_MODEL_PREDICT, NULL);
	remove_proc_entry(PROC_MODEL_LOAD, NULL);
	mutex_lock(&cuda_lock);
	cuda_shutting_down = true;
	ml_reset_cuda_request_locked();
	mutex_unlock(&cuda_lock);
	wake_up_interruptible_all(&cuda_request_wq);
	wake_up_all(&cuda_result_wq);
	ml_model_free(&global_model);
	mutex_lock(&model_blob_lock);
	old_blob = model_blob;
	model_blob = NULL;
	model_blob_size = 0;
	mutex_unlock(&model_blob_lock);
	kfree(old_blob);

	pr_info("kernel-ml: Unloaded. Performed %llu inferences\n",
	        (unsigned long long)atomic64_read(&inference_count));
}

module_init(kernel_ml_init);
module_exit(kernel_ml_exit);

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Agent eBPF Filter");
MODULE_DESCRIPTION("Kernel-space ML inference engine with optional CUDA userspace offload");
MODULE_VERSION("1.1");
