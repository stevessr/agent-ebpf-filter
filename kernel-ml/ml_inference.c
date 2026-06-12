/* Kernel ML Inference Engine - Core Implementation */

#include <linux/kernel.h>
#include <linux/module.h>
#include <linux/slab.h>
#include <linux/uaccess.h>
#include "ml_inference.h"

/* Decision tree traversal - pure integer math */
static enum ml_action traverse_tree(const struct tree_node *nodes,
                                    size_t num_nodes,
                                    const struct feature_vector *fv,
                                    u32 max_depth)
{
	s32 idx = 0;
	u32 depth = 0;

	if (max_depth == 0 || max_depth > ML_MAX_TREE_DEPTH)
		max_depth = ML_DEFAULT_MAX_DEPTH;

	while (depth < max_depth && idx >= 0 && idx < num_nodes) {
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
	int votes[ML_MAX_CLASSES] = {0};
	int i;
	u32 num_classes;
	enum ml_action best = ML_ACTION_ALLOW;

	if (!model || !fv || model->num_trees == 0)
		return ML_ACTION_ALLOW;

	num_classes = model->num_classes;
	if (num_classes == 0)
		num_classes = ML_DEFAULT_NUM_CLASSES;
	if (num_classes > ML_MAX_CLASSES)
		num_classes = ML_MAX_CLASSES;

	/* Query each tree */
	for (i = 0; i < model->num_trees && i < NUM_TREES; i++) {
		if (!model->trees[i])
			continue;

		enum ml_action action = traverse_tree(
			model->trees[i],
			model->tree_sizes[i] / sizeof(struct tree_node),
			fv,
			model->max_depth
		);

		if ((u32)action < num_classes)
			votes[action]++;
	}

	/* Return majority. Ties stay with the lowest class for stable behavior. */
	for (i = 1; i < num_classes; i++) {
		if (votes[i] > votes[best])
			best = (enum ml_action)i;
	}

	return best;
}

/* Load model from userspace */
int ml_model_load(struct ml_model *model, const void __user *data, size_t size)
{
	int ret = -EINVAL;
	u32 header[4];
	u32 ext_header[2];
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
	model->num_classes = ML_DEFAULT_NUM_CLASSES;
	model->max_depth = ML_DEFAULT_MAX_DEPTH;

	if (model->num_trees > NUM_TREES || model->feature_dim != FEATURE_DIM)
		return -EINVAL;

	offset = sizeof(header);

	/* v2+ model header extends v1 with: num_classes, max_depth. */
	if (model->version >= 2) {
		if (size < offset + sizeof(ext_header))
			return -EINVAL;
		if (copy_from_user(ext_header, (u8 __user *)data + offset, sizeof(ext_header)))
			return -EFAULT;
		model->num_classes = ext_header[0];
		model->max_depth = ext_header[1] ? ext_header[1] : ML_DEFAULT_MAX_DEPTH;
		offset += sizeof(ext_header);
	}

	if (model->num_classes == 0 || model->num_classes > ML_MAX_CLASSES)
		return -EINVAL;
	if (model->max_depth == 0 || model->max_depth > ML_MAX_TREE_DEPTH)
		return -EINVAL;

	/* Load each tree */
	for (i = 0; i < model->num_trees; i++) {
		u32 tree_nodes;
		size_t tree_size;

		if (offset + sizeof(u32) > size)
			goto cleanup;

		if (copy_from_user(&tree_nodes, (u8 __user *)data + offset, sizeof(u32)))
			goto cleanup;

		offset += sizeof(u32);
		if (tree_nodes == 0 || tree_nodes > MAX_TREE_NODES)
			goto cleanup;
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

	pr_info("kernel-ml: Loaded model v%u: %u trees, %u features, %u classes, max_depth=%u\n",
	        model->version, model->num_trees, model->feature_dim,
	        model->num_classes, model->max_depth);

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
