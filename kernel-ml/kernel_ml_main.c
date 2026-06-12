/* Kernel ML Module - Main Entry Point */

#include <linux/module.h>
#include <linux/kernel.h>
#include <linux/init.h>
#include <linux/proc_fs.h>
#include <linux/seq_file.h>
#include <linux/uaccess.h>
#include "ml_inference.h"

#define MODULE_NAME "kernel_ml"
#define PROC_MODEL_LOAD "ml_load"
#define PROC_MODEL_PREDICT "ml_predict"
#define PROC_MODEL_STATS "ml_stats"

static struct ml_model global_model;
static atomic64_t inference_count = ATOMIC64_INIT(0);
static atomic64_t block_count = ATOMIC64_INIT(0);
static atomic64_t alert_count = ATOMIC64_INIT(0);

/* Proc: Load model from userspace */
static ssize_t proc_model_load_write(struct file *file, const char __user *buffer,
                                     size_t count, loff_t *pos)
{
	int ret;

	if (count > 10 * 1024 * 1024)
		return -EINVAL;

	ml_model_free(&global_model);
	ret = ml_model_load(&global_model, buffer, count);

	return ret ? ret : count;
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

	action = ml_inference(&global_model, &fv);
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
	seq_printf(m, "Total Inferences: %llu\n", atomic64_read(&inference_count));
	seq_printf(m, "Blocks: %llu\n", atomic64_read(&block_count));
	seq_printf(m, "Alerts: %llu\n", atomic64_read(&alert_count));
	return 0;
}

static int proc_stats_open(struct inode *inode, struct file *file)
{
	return single_open(file, proc_model_stats_show, NULL);
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

static int __init kernel_ml_init(void)
{
	struct proc_dir_entry *entry;

	memset(&global_model, 0, sizeof(global_model));

	entry = proc_create(PROC_MODEL_LOAD, 0200, NULL, &proc_load_ops);
	if (!entry)
		goto err_load;

	entry = proc_create(PROC_MODEL_PREDICT, 0200, NULL, &proc_predict_ops);
	if (!entry)
		goto err_predict;

	entry = proc_create(PROC_MODEL_STATS, 0444, NULL, &proc_stats_ops);
	if (!entry)
		goto err_stats;

	pr_info("kernel-ml: Loaded. Interface: /proc/%s, /proc/%s, /proc/%s\n",
	        PROC_MODEL_LOAD, PROC_MODEL_PREDICT, PROC_MODEL_STATS);

	return 0;

err_stats:
	remove_proc_entry(PROC_MODEL_PREDICT, NULL);
err_predict:
	remove_proc_entry(PROC_MODEL_LOAD, NULL);
err_load:
	return -ENOMEM;
}

static void __exit kernel_ml_exit(void)
{
	remove_proc_entry(PROC_MODEL_STATS, NULL);
	remove_proc_entry(PROC_MODEL_PREDICT, NULL);
	remove_proc_entry(PROC_MODEL_LOAD, NULL);
	ml_model_free(&global_model);

	pr_info("kernel-ml: Unloaded. Performed %llu inferences\n",
	        atomic64_read(&inference_count));
}

module_init(kernel_ml_init);
module_exit(kernel_ml_exit);

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Agent eBPF Filter");
MODULE_DESCRIPTION("Kernel-space ML inference engine for behavior classification");
MODULE_VERSION("1.0");
