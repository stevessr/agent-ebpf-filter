/* Kernel ML Module - Main Entry Point */

#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/init.h>
#include <linux/jiffies.h>
#include <linux/proc_fs.h>
#include <linux/poll.h>
#include <linux/seq_file.h>
#include <linux/slab.h>
#include <linux/string.h>
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
module_param_named(backend, backend_param, charp, 0644);
MODULE_PARM_DESC(backend, "Inference backend: kernel, cuda, or auto");
module_param(cuda_timeout_ms, uint, 0644);
MODULE_PARM_DESC(cuda_timeout_ms, "CUDA offload timeout in milliseconds");

static enum ml_backend_type active_backend = ML_BACKEND_KERNEL;

static DEFINE_MUTEX(model_blob_lock);
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
static struct ml_cuda_request cuda_request;
static struct ml_cuda_result cuda_result;

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
	} else if (cuda_result.action > ML_ACTION_ALERT) {
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

	if (active_backend == ML_BACKEND_KERNEL)
		return ml_inference(model, fv);

	if (active_backend == ML_BACKEND_AUTO && atomic_read(&cuda_clients) <= 0)
		return ml_inference(model, fv);

	ret = ml_cuda_inference(fv, &action);
	if (!ret)
		return action;

	atomic64_inc(&cuda_fallback_count);
	return ml_inference(model, fv);
}

/* Proc: Load model from userspace */
static ssize_t proc_model_load_write(struct file *file, const char __user *buffer,
                                     size_t count, loff_t *pos)
{
	void *raw;
	int ret;

	if (count > 10 * 1024 * 1024)
		return -EINVAL;

	raw = memdup_user(buffer, count);
	if (IS_ERR(raw))
		return PTR_ERR(raw);

	ml_model_free(&global_model);
	ret = ml_model_load(&global_model, buffer, count);
	if (ret) {
		kfree(raw);
		return ret;
	}

	ml_store_model_blob(raw, count);
	return count;
}

/* Proc: Perform inference */
static ssize_t proc_model_predict_write(struct file *file, const char __user *buffer,
                                        size_t count, loff_t *pos)
{
	struct feature_vector fv;
	enum ml_action action;

	if (count != sizeof(fv))
		return -EINVAL;

	if (copy_from_user(&fv, buffer, sizeof(fv)))
		return -EFAULT;

	action = ml_run_inference(&global_model, &fv);
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
	        action == ML_ACTION_BLOCK ? "BLOCK" :
	        action == ML_ACTION_ALERT ? "ALERT" : "ALLOW");

	return count;
}

/* Proc: Stats */
static int proc_model_stats_show(struct seq_file *m, void *v)
{
	seq_printf(m, "Model Version: %u\n", global_model.version);
	seq_printf(m, "Trees: %u\n", global_model.num_trees);
	seq_printf(m, "Features: %u\n", global_model.feature_dim);
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
		ret = wait_event_interruptible(cuda_request_wq, cuda_request_pending);
		if (ret)
			return ret;
	}

	mutex_lock(&cuda_lock);
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
	if (result.status == 0 && result.action > ML_ACTION_ALERT)
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

	pr_info("kernel-ml: Loaded. Interface: /proc/%s, /proc/%s, /proc/%s, backend=%s\n",
	        PROC_MODEL_LOAD, PROC_MODEL_PREDICT, PROC_MODEL_STATS,
	        ml_backend_name(active_backend));

	return 0;

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

	remove_proc_entry(PROC_CUDA_MODEL, NULL);
	remove_proc_entry(PROC_CUDA_RESULT, NULL);
	remove_proc_entry(PROC_CUDA_REQUEST, NULL);
	remove_proc_entry(PROC_MODEL_BACKEND, NULL);
	remove_proc_entry(PROC_MODEL_STATS, NULL);
	remove_proc_entry(PROC_MODEL_PREDICT, NULL);
	remove_proc_entry(PROC_MODEL_LOAD, NULL);
	mutex_lock(&cuda_lock);
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
