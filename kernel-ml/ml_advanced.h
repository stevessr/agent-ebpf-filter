/* Extended ML Models - Decision Tree, KNN, Naive Bayes, Gradient Boosting, Ensemble
 *
 * Additional model types for comprehensive ML coverage
 */

#ifndef _KERNEL_ML_ADVANCED_H
#define _KERNEL_ML_ADVANCED_H

#include "ml_inference.h"
#include "ml_models.h"

/* ===== Decision Tree (Single Tree) ===== */
struct decision_tree_model {
	u32 version;
	u32 num_nodes;
	struct tree_node *nodes;
};

enum ml_action dt_inference(struct decision_tree_model *model, struct feature_vector *fv);
int dt_model_load(struct decision_tree_model *model, const void __user *data, size_t size);
void dt_model_free(struct decision_tree_model *model);

/* ===== K-Nearest Neighbors ===== */
#define KNN_MAX_NEIGHBORS 100
#define KNN_MAX_K 15

struct knn_sample {
	s64 features[FEATURE_DIM];
	u8 label;
};

struct knn_model {
	u32 version;
	u32 num_samples;
	u32 k;  /* Number of neighbors */
	struct knn_sample samples[KNN_MAX_NEIGHBORS];
};

enum ml_action knn_inference(struct knn_model *model, struct feature_vector *fv);
int knn_model_load(struct knn_model *model, const void __user *data, size_t size);
void knn_model_free(struct knn_model *model);

/* ===== Naive Bayes ===== */
struct nb_model {
	u32 version;
	u32 feature_dim;
	s64 class_priors[3];              /* P(y=c) for ALLOW/BLOCK/ALERT */
	s64 feature_means[3][FEATURE_DIM]; /* μ for each class */
	s64 feature_stds[3][FEATURE_DIM];  /* σ for each class */
};

enum ml_action nb_inference(struct nb_model *model, struct feature_vector *fv);
int nb_model_load(struct nb_model *model, const void __user *data, size_t size);
void nb_model_free(struct nb_model *model);

/* ===== Gradient Boosting (Simplified) ===== */
#define GB_MAX_TREES 50

struct gb_tree {
	u32 num_nodes;
	struct tree_node *nodes;
	s64 learning_rate;
};

struct gb_model {
	u32 version;
	u32 num_trees;
	s64 base_score[3];
	struct gb_tree trees[GB_MAX_TREES];
};

enum ml_action gb_inference(struct gb_model *model, struct feature_vector *fv);
int gb_model_load(struct gb_model *model, const void __user *data, size_t size);
void gb_model_free(struct gb_model *model);

/* ===== Ensemble (Model Voting) ===== */
#define ENSEMBLE_MAX_MODELS 5

enum ensemble_strategy {
	ENSEMBLE_VOTING_HARD = 0,
	ENSEMBLE_VOTING_SOFT = 1,
	ENSEMBLE_WEIGHTED = 2,
};

struct ensemble_model {
	u32 version;
	u32 num_models;
	enum ensemble_strategy strategy;
	s64 weights[ENSEMBLE_MAX_MODELS];
	struct unified_model models[ENSEMBLE_MAX_MODELS];
};

enum ml_action ensemble_inference(struct ensemble_model *model, struct feature_vector *fv);
int ensemble_model_load(struct ensemble_model *model, const void __user *data, size_t size);
void ensemble_model_free(struct ensemble_model *model);

/* ===== Helper Functions ===== */

/* Euclidean distance (for KNN) */
static inline s64 euclidean_distance(const s64 *a, const s64 *b, u32 dim)
{
	s64 sum = 0;
	u32 i;

	for (i = 0; i < dim; i++) {
		s64 diff = a[i] - b[i];
		sum += (diff * diff) / FLOAT_SCALE;
	}

	/* Return sqrt approximation: Newton's method */
	if (sum == 0) return 0;
	s64 x = sum / 2;
	x = (x + sum / x) / 2;  /* 1 iteration */
	x = (x + sum / x) / 2;  /* 2 iterations */
	return x;
}

/* Gaussian PDF approximation (for Naive Bayes) */
static inline s64 gaussian_pdf(s64 x, s64 mean, s64 std)
{
	if (std <= 0) std = FLOAT_SCALE;  /* Avoid division by zero */

	s64 diff = x - mean;
	s64 var = (std * std) / FLOAT_SCALE;
	s64 exp_term = -(diff * diff) / (2 * var);

	/* e^x approximation for small x: 1 + x + x^2/2 */
	s64 exp_approx = FLOAT_SCALE + exp_term + (exp_term * exp_term) / (2 * FLOAT_SCALE);

	/* Normalize: 1/(sqrt(2π)σ) ≈ 400/σ */
	return (400 * exp_approx) / std;
}

/* Extended unified model */
enum advanced_model_type {
	AMODEL_DECISION_TREE = 10,
	AMODEL_KNN = 11,
	AMODEL_NAIVE_BAYES = 12,
	AMODEL_GRADIENT_BOOSTING = 13,
	AMODEL_ENSEMBLE = 14,
};

struct advanced_unified_model {
	enum advanced_model_type type;
	union {
		struct decision_tree_model dt;
		struct knn_model knn;
		struct nb_model nb;
		struct gb_model gb;
		struct ensemble_model ensemble;
	} data;
};

enum ml_action advanced_inference(struct advanced_unified_model *model, struct feature_vector *fv);
int advanced_model_load(struct advanced_unified_model *model, enum advanced_model_type type,
                        const void __user *data, size_t size);
void advanced_model_free(struct advanced_unified_model *model);

#endif /* _KERNEL_ML_ADVANCED_H */
