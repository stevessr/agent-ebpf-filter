/* Advanced ML Model Implementations */

#include <linux/kernel.h>
#include <linux/module.h>
#include <linux/slab.h>
#include <linux/uaccess.h>
#include <linux/sort.h>
#include "ml_advanced.h"

/* ===== Decision Tree ===== */

enum ml_action dt_inference(struct decision_tree_model *model, struct feature_vector *fv)
{
	s32 idx = 0;
	int depth = 0;

	if (!model || !fv || !model->nodes)
		return ML_ACTION_ALLOW;

	while (depth < 64 && idx >= 0 && idx < model->num_nodes) {
		const struct tree_node *node = &model->nodes[idx];

		if (node->is_leaf)
			return (enum ml_action)node->leaf_value;

		if (node->feature_idx >= FEATURE_DIM)
			break;

		s64 feature_val = fv->features[node->feature_idx];
		idx = (feature_val < node->threshold) ? node->left_child : node->right_child;
		depth++;
	}

	return ML_ACTION_ALLOW;
}

int dt_model_load(struct decision_tree_model *model, const void __user *data, size_t size)
{
	u32 header[2];
	size_t tree_size;

	if (!model || !data || size < sizeof(header))
		return -EINVAL;

	if (copy_from_user(header, data, sizeof(header)))
		return -EFAULT;

	model->version = header[0];
	model->num_nodes = header[1];

	tree_size = model->num_nodes * sizeof(struct tree_node);
	if (size < sizeof(header) + tree_size)
		return -EINVAL;

	model->nodes = kmalloc(tree_size, GFP_KERNEL);
	if (!model->nodes)
		return -ENOMEM;

	if (copy_from_user(model->nodes, (u8 __user *)data + sizeof(header), tree_size)) {
		kfree(model->nodes);
		return -EFAULT;
	}

	pr_info("kernel-ml: Loaded DT model v%u: %u nodes\n",
	        model->version, model->num_nodes);

	return 0;
}

void dt_model_free(struct decision_tree_model *model)
{
	if (model && model->nodes) {
		kfree(model->nodes);
		memset(model, 0, sizeof(*model));
	}
}

/* ===== K-Nearest Neighbors ===== */

struct knn_neighbor {
	s64 distance;
	u8 label;
};

static int knn_neighbor_cmp(const void *a, const void *b)
{
	const struct knn_neighbor *na = a;
	const struct knn_neighbor *nb = b;
	return (na->distance > nb->distance) - (na->distance < nb->distance);
}

enum ml_action knn_inference(struct knn_model *model, struct feature_vector *fv)
{
	struct knn_neighbor neighbors[KNN_MAX_NEIGHBORS];
	int votes[3] = {0, 0, 0};
	u32 i;

	if (!model || !fv || model->k == 0 || model->k > KNN_MAX_K)
		return ML_ACTION_ALLOW;

	/* Compute distances */
	for (i = 0; i < model->num_samples && i < KNN_MAX_NEIGHBORS; i++) {
		neighbors[i].distance = euclidean_distance(
			fv->features,
			model->samples[i].features,
			FEATURE_DIM
		);
		neighbors[i].label = model->samples[i].label;
	}

	/* Sort by distance */
	sort(neighbors, model->num_samples, sizeof(struct knn_neighbor),
	     knn_neighbor_cmp, NULL);

	/* Vote with K nearest */
	for (i = 0; i < model->k && i < model->num_samples; i++) {
		if (neighbors[i].label < 3)
			votes[neighbors[i].label]++;
	}

	/* Return majority */
	if (votes[ML_ACTION_BLOCK] > votes[ML_ACTION_ALLOW] &&
	    votes[ML_ACTION_BLOCK] > votes[ML_ACTION_ALERT])
		return ML_ACTION_BLOCK;
	if (votes[ML_ACTION_ALERT] > votes[ML_ACTION_ALLOW])
		return ML_ACTION_ALERT;
	return ML_ACTION_ALLOW;
}

int knn_model_load(struct knn_model *model, const void __user *data, size_t size)
{
	u32 header[3];
	size_t samples_size;

	if (!model || !data || size < sizeof(header))
		return -EINVAL;

	if (copy_from_user(header, data, sizeof(header)))
		return -EFAULT;

	model->version = header[0];
	model->num_samples = header[1];
	model->k = header[2];

	if (model->num_samples > KNN_MAX_NEIGHBORS || model->k > KNN_MAX_K)
		return -EINVAL;

	samples_size = model->num_samples * sizeof(struct knn_sample);
	if (size < sizeof(header) + samples_size)
		return -EINVAL;

	if (copy_from_user(model->samples, (u8 __user *)data + sizeof(header), samples_size))
		return -EFAULT;

	pr_info("kernel-ml: Loaded KNN model v%u: %u samples, k=%u\n",
	        model->version, model->num_samples, model->k);

	return 0;
}

void knn_model_free(struct knn_model *model)
{
	if (model)
		memset(model, 0, sizeof(*model));
}

/* ===== Naive Bayes ===== */

enum ml_action nb_inference(struct nb_model *model, struct feature_vector *fv)
{
	s64 log_probs[3];
	u32 i, c;

	if (!model || !fv)
		return ML_ACTION_ALLOW;

	/* Compute log P(y=c | x) for each class */
	for (c = 0; c < 3; c++) {
		log_probs[c] = model->class_priors[c];

		for (i = 0; i < FEATURE_DIM; i++) {
			s64 pdf = gaussian_pdf(
				fv->features[i],
				model->feature_means[c][i],
				model->feature_stds[c][i]
			);
			/* log(pdf) approximation: just use pdf directly (simplified) */
			log_probs[c] += pdf;
		}
	}

	return (enum ml_action)argmax(log_probs, 3);
}

int nb_model_load(struct nb_model *model, const void __user *data, size_t size)
{
	u32 header[2];
	size_t offset;

	if (!model || !data || size < sizeof(header))
		return -EINVAL;

	if (copy_from_user(header, data, sizeof(header)))
		return -EFAULT;

	model->version = header[0];
	model->feature_dim = header[1];

	if (model->feature_dim != FEATURE_DIM)
		return -EINVAL;

	offset = sizeof(header);

	/* Load priors */
	if (copy_from_user(model->class_priors, (u8 __user *)data + offset, sizeof(s64) * 3))
		return -EFAULT;
	offset += sizeof(s64) * 3;

	/* Load means */
	if (copy_from_user(model->feature_means, (u8 __user *)data + offset,
	                   sizeof(s64) * 3 * FEATURE_DIM))
		return -EFAULT;
	offset += sizeof(s64) * 3 * FEATURE_DIM;

	/* Load stds */
	if (copy_from_user(model->feature_stds, (u8 __user *)data + offset,
	                   sizeof(s64) * 3 * FEATURE_DIM))
		return -EFAULT;

	pr_info("kernel-ml: Loaded NB model v%u: %u features\n",
	        model->version, model->feature_dim);

	return 0;
}

void nb_model_free(struct nb_model *model)
{
	if (model)
		memset(model, 0, sizeof(*model));
}

/* ===== Gradient Boosting ===== */

enum ml_action gb_inference(struct gb_model *model, struct feature_vector *fv)
{
	s64 scores[3];
	u32 i;

	if (!model || !fv)
		return ML_ACTION_ALLOW;

	/* Initialize with base scores */
	for (i = 0; i < 3; i++)
		scores[i] = model->base_score[i];

	/* Accumulate predictions from each tree */
	for (i = 0; i < model->num_trees && i < GB_MAX_TREES; i++) {
		if (!model->trees[i].nodes)
			continue;

		/* Traverse tree */
		s32 idx = 0;
		int depth = 0;
		while (depth < 64 && idx >= 0 && idx < model->trees[i].num_nodes) {
			const struct tree_node *node = &model->trees[i].nodes[idx];

			if (node->is_leaf) {
				/* Add weighted prediction */
				scores[node->leaf_value] += model->trees[i].learning_rate;
				break;
			}

			if (node->feature_idx >= FEATURE_DIM)
				break;

			s64 feature_val = fv->features[node->feature_idx];
			idx = (feature_val < node->threshold) ? node->left_child : node->right_child;
			depth++;
		}
	}

	return (enum ml_action)argmax(scores, 3);
}

int gb_model_load(struct gb_model *model, const void __user *data, size_t size)
{
	/* Simplified: not fully implemented due to complexity */
	pr_warn("kernel-ml: GB model loading not fully implemented\n");
	return -ENOSYS;
}

void gb_model_free(struct gb_model *model)
{
	u32 i;

	if (!model)
		return;

	for (i = 0; i < GB_MAX_TREES; i++) {
		if (model->trees[i].nodes) {
			kfree(model->trees[i].nodes);
			model->trees[i].nodes = NULL;
		}
	}

	memset(model, 0, sizeof(*model));
}

/* ===== Ensemble ===== */

enum ml_action ensemble_inference(struct ensemble_model *model, struct feature_vector *fv)
{
	s64 votes[3] = {0, 0, 0};
	u32 i;

	if (!model || !fv)
		return ML_ACTION_ALLOW;

	for (i = 0; i < model->num_models && i < ENSEMBLE_MAX_MODELS; i++) {
		enum ml_action action = unified_inference(&model->models[i], fv);

		if (model->strategy == ENSEMBLE_WEIGHTED)
			votes[action] += model->weights[i];
		else
			votes[action] += FLOAT_SCALE;
	}

	return (enum ml_action)argmax(votes, 3);
}

int ensemble_model_load(struct ensemble_model *model, const void __user *data, size_t size)
{
	pr_warn("kernel-ml: Ensemble model loading not fully implemented\n");
	return -ENOSYS;
}

void ensemble_model_free(struct ensemble_model *model)
{
	u32 i;

	if (!model)
		return;

	for (i = 0; i < ENSEMBLE_MAX_MODELS; i++)
		unified_model_free(&model->models[i]);

	memset(model, 0, sizeof(*model));
}

/* ===== Advanced Unified Interface ===== */

enum ml_action advanced_inference(struct advanced_unified_model *model, struct feature_vector *fv)
{
	if (!model || !fv)
		return ML_ACTION_ALLOW;

	switch (model->type) {
	case AMODEL_DECISION_TREE:
		return dt_inference(&model->data.dt, fv);
	case AMODEL_KNN:
		return knn_inference(&model->data.knn, fv);
	case AMODEL_NAIVE_BAYES:
		return nb_inference(&model->data.nb, fv);
	case AMODEL_GRADIENT_BOOSTING:
		return gb_inference(&model->data.gb, fv);
	case AMODEL_ENSEMBLE:
		return ensemble_inference(&model->data.ensemble, fv);
	default:
		return ML_ACTION_ALLOW;
	}
}

int advanced_model_load(struct advanced_unified_model *model, enum advanced_model_type type,
                        const void __user *data, size_t size)
{
	if (!model)
		return -EINVAL;

	advanced_model_free(model);
	model->type = type;

	switch (type) {
	case AMODEL_DECISION_TREE:
		return dt_model_load(&model->data.dt, data, size);
	case AMODEL_KNN:
		return knn_model_load(&model->data.knn, data, size);
	case AMODEL_NAIVE_BAYES:
		return nb_model_load(&model->data.nb, data, size);
	case AMODEL_GRADIENT_BOOSTING:
		return gb_model_load(&model->data.gb, data, size);
	case AMODEL_ENSEMBLE:
		return ensemble_model_load(&model->data.ensemble, data, size);
	default:
		return -EINVAL;
	}
}

void advanced_model_free(struct advanced_unified_model *model)
{
	if (!model)
		return;

	switch (model->type) {
	case AMODEL_DECISION_TREE:
		dt_model_free(&model->data.dt);
		break;
	case AMODEL_KNN:
		knn_model_free(&model->data.knn);
		break;
	case AMODEL_NAIVE_BAYES:
		nb_model_free(&model->data.nb);
		break;
	case AMODEL_GRADIENT_BOOSTING:
		gb_model_free(&model->data.gb);
		break;
	case AMODEL_ENSEMBLE:
		ensemble_model_free(&model->data.ensemble);
		break;
	}

	memset(model, 0, sizeof(*model));
}

EXPORT_SYMBOL_GPL(dt_inference);
EXPORT_SYMBOL_GPL(knn_inference);
EXPORT_SYMBOL_GPL(nb_inference);
EXPORT_SYMBOL_GPL(gb_inference);
EXPORT_SYMBOL_GPL(ensemble_inference);
EXPORT_SYMBOL_GPL(advanced_inference);
EXPORT_SYMBOL_GPL(advanced_model_load);
EXPORT_SYMBOL_GPL(advanced_model_free);
