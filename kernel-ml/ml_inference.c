/* Kernel ML Inference Engine - Core Implementation */

#include <linux/kernel.h>
#include <linux/module.h>
#include <linux/slab.h>
#include <linux/uaccess.h>
#include "ml_inference.h"

/* Decision tree traversal - pure integer math */
static enum ml_action traverse_tree(const struct tree_node *nodes,
                                    size_t num_nodes,
                                    const struct feature_vector *fv)
{
	s32 idx = 0;
	int depth = 0;

	while (depth < 64 && idx >= 0 && idx < num_nodes) {
		const struct tree_node *node = &nodes[idx];

		if (node->is_leaf)
			return (enum ml_action)node->leaf_value;

		/* Branch based on feature comparison */
		if (node->feature_idx >= FEATURE_DIM)
			break;

		s64 feature_val = fv->features[node->feature_idx];

		if (feature_val < node->threshold)
			idx = node->left_child;
		else
			idx = node->right_child;

		depth++;
	}

	return ML_ACTION_ALLOW; /* Default on error */
}

/* Random forest majority vote */
enum ml_action ml_inference(struct ml_model *model, struct feature_vector *fv)
{
	int votes[3] = {0, 0, 0};
	int i;

	if (!model || !fv || model->num_trees == 0)
		return ML_ACTION_ALLOW;

	/* Query each tree */
	for (i = 0; i < model->num_trees && i < NUM_TREES; i++) {
		if (!model->trees[i])
			continue;

		enum ml_action action = traverse_tree(
			model->trees[i],
			model->tree_sizes[i] / sizeof(struct tree_node),
			fv
		);

		if (action < 3)
			votes[action]++;
	}

	/* Return majority */
	if (votes[ML_ACTION_BLOCK] > votes[ML_ACTION_ALLOW] &&
	    votes[ML_ACTION_BLOCK] > votes[ML_ACTION_ALERT])
		return ML_ACTION_BLOCK;

	if (votes[ML_ACTION_ALERT] > votes[ML_ACTION_ALLOW])
		return ML_ACTION_ALERT;

	return ML_ACTION_ALLOW;
}

/* Load model from userspace */
int ml_model_load(struct ml_model *model, const void __user *data, size_t size)
{
	int ret = -EINVAL;
	u32 header[4];
	size_t offset = 0;
	int i;

	if (!model || !data || size < sizeof(header))
		return -EINVAL;

	/* Read header: version, num_trees, feature_dim, total_nodes */
	if (copy_from_user(header, data, sizeof(header)))
		return -EFAULT;

	model->version = header[0];
	model->num_trees = header[1];
	model->feature_dim = header[2];
	model->total_nodes = header[3];

	if (model->num_trees > NUM_TREES || model->feature_dim != FEATURE_DIM)
		return -EINVAL;

	offset = sizeof(header);

	/* Load each tree */
	for (i = 0; i < model->num_trees; i++) {
		u32 tree_nodes;
		size_t tree_size;

		if (offset + sizeof(u32) > size)
			goto cleanup;

		if (copy_from_user(&tree_nodes, (u8 __user *)data + offset, sizeof(u32)))
			goto cleanup;

		offset += sizeof(u32);
		tree_size = tree_nodes * sizeof(struct tree_node);

		if (offset + tree_size > size)
			goto cleanup;

		model->trees[i] = kmalloc(tree_size, GFP_KERNEL);
		if (!model->trees[i])
			goto cleanup;

		if (copy_from_user(model->trees[i], (u8 __user *)data + offset, tree_size)) {
			kfree(model->trees[i]);
			model->trees[i] = NULL;
			goto cleanup;
		}

		model->tree_sizes[i] = tree_size;
		offset += tree_size;
	}

	pr_info("kernel-ml: Loaded model v%u: %u trees, %u features\n",
	        model->version, model->num_trees, model->feature_dim);

	return 0;

cleanup:
	for (i = 0; i < NUM_TREES; i++) {
		if (model->trees[i]) {
			kfree(model->trees[i]);
			model->trees[i] = NULL;
		}
	}
	return ret;
}

void ml_model_free(struct ml_model *model)
{
	int i;

	if (!model)
		return;

	for (i = 0; i < NUM_TREES; i++) {
		if (model->trees[i]) {
			kfree(model->trees[i]);
			model->trees[i] = NULL;
		}
	}

	memset(model, 0, sizeof(*model));
}

/* Feature extraction from syscall */
void extract_features(struct feature_vector *fv,
                      u32 syscall_nr,
                      u32 pid,
                      const char *comm,
                      const long *args)
{
	int i;

	memset(fv, 0, sizeof(*fv));

	fv->pid = pid;
	fv->syscall_type = syscall_nr;
	if (comm)
		strscpy(fv->comm, comm, sizeof(fv->comm));

	/* Feature 0-5: syscall args (normalized) */
	for (i = 0; i < 6 && args; i++)
		fv->features[i] = FLOAT_TO_FIXED((args[i] / 1000000));

	/* Feature 6: syscall number */
	fv->features[6] = FLOAT_TO_FIXED(syscall_nr);

	/* Feature 7: PID */
	fv->features[7] = FLOAT_TO_FIXED(pid);

	/* Features 8-127: Reserved for future use */
}

EXPORT_SYMBOL_GPL(ml_inference);
EXPORT_SYMBOL_GPL(ml_model_load);
EXPORT_SYMBOL_GPL(ml_model_free);
EXPORT_SYMBOL_GPL(extract_features);
