/* Extended Model Implementations - SVM, LR, Neural Network */

#include <linux/kernel.h>
#include <linux/module.h>
#include <linux/slab.h>
#include <linux/uaccess.h>
#include "ml_models.h"

/* ===== SVM Implementation ===== */

/* Linear kernel: K(x, y) = x · y */
static s64 dot_product(const s64 *a, const s64 *b, u32 dim)
{
	s64 result = 0;
	u32 i;

	for (i = 0; i < dim; i++)
		result += (a[i] * b[i]) / FLOAT_SCALE;  /* Rescale after multiply */

	return result;
}

enum ml_action svm_inference(struct svm_model *model, struct feature_vector *fv)
{
	s64 decision = model->bias;

	if (!model || !fv)
		return ML_ACTION_ALLOW;

	/* Linear SVM: decision = w·x + b */
	decision += dot_product(model->weights, fv->features, model->feature_dim);

	/* Decision boundaries: negative = BLOCK, positive = ALLOW, near-zero = ALERT */
	if (decision < -500)
		return ML_ACTION_BLOCK;
	if (decision > 500)
		return ML_ACTION_ALLOW;
	return ML_ACTION_ALERT;
}

int svm_model_load(struct svm_model *model, const void __user *data, size_t size)
{
	u32 header[4];
	size_t offset = 0;
	size_t expected_size;

	if (!model || !data || size < sizeof(header))
		return -EINVAL;

	if (copy_from_user(header, data, sizeof(header)))
		return -EFAULT;

	model->version = header[0];
	model->num_support_vectors = header[1];
	model->feature_dim = header[2];

	if (model->feature_dim != FEATURE_DIM)
		return -EINVAL;

	offset = sizeof(header);
	expected_size = offset + sizeof(s64) +  /* bias */
	                sizeof(s64) * FEATURE_DIM +  /* weights */
	                sizeof(s64) * model->num_support_vectors * FEATURE_DIM +  /* support vectors */
	                sizeof(s64) * model->num_support_vectors;  /* alphas */

	if (size < expected_size)
		return -EINVAL;

	/* Load bias */
	if (copy_from_user(&model->bias, (u8 __user *)data + offset, sizeof(s64)))
		return -EFAULT;
	offset += sizeof(s64);

	/* Load weights */
	if (copy_from_user(model->weights, (u8 __user *)data + offset,
	                   sizeof(s64) * FEATURE_DIM))
		return -EFAULT;

	pr_info("kernel-ml: Loaded SVM model v%u: %u features\n",
	        model->version, model->feature_dim);

	return 0;
}

void svm_model_free(struct svm_model *model)
{
	if (model)
		memset(model, 0, sizeof(*model));
}

/* ===== Logistic Regression Implementation ===== */

enum ml_action lr_inference(struct lr_model *model, struct feature_vector *fv)
{
	s64 logit, prob;

	if (!model || !fv)
		return ML_ACTION_ALLOW;

	/* Compute logit: z = w·x + b */
	logit = model->bias + dot_product(model->weights, fv->features, model->feature_dim);

	/* Approximate sigmoid */
	prob = sigmoid_approx(logit);

	/* Threshold-based classification */
	if (prob < model->thresholds[0])
		return ML_ACTION_BLOCK;
	if (prob > model->thresholds[1])
		return ML_ACTION_ALLOW;
	return ML_ACTION_ALERT;
}

int lr_model_load(struct lr_model *model, const void __user *data, size_t size)
{
	u32 header[2];
	size_t offset = 0;
	size_t expected_size;

	if (!model || !data || size < sizeof(header))
		return -EINVAL;

	if (copy_from_user(header, data, sizeof(header)))
		return -EFAULT;

	model->version = header[0];
	model->feature_dim = header[1];

	if (model->feature_dim != FEATURE_DIM)
		return -EINVAL;

	offset = sizeof(header);
	expected_size = offset + sizeof(s64) * FEATURE_DIM + sizeof(s64) + sizeof(s64) * 2;

	if (size < expected_size)
		return -EINVAL;

	/* Load weights */
	if (copy_from_user(model->weights, (u8 __user *)data + offset,
	                   sizeof(s64) * FEATURE_DIM))
		return -EFAULT;
	offset += sizeof(s64) * FEATURE_DIM;

	/* Load bias */
	if (copy_from_user(&model->bias, (u8 __user *)data + offset, sizeof(s64)))
		return -EFAULT;
	offset += sizeof(s64);

	/* Load thresholds */
	if (copy_from_user(model->thresholds, (u8 __user *)data + offset, sizeof(s64) * 2))
		return -EFAULT;

	pr_info("kernel-ml: Loaded LR model v%u: %u features\n",
	        model->version, model->feature_dim);

	return 0;
}

void lr_model_free(struct lr_model *model)
{
	if (model)
		memset(model, 0, sizeof(*model));
}

/* ===== Neural Network Implementation ===== */

enum ml_action nn_inference(struct nn_model *model, struct feature_vector *fv)
{
	s64 hidden[NN_MAX_HIDDEN];
	s64 output[3];
	u32 i, j;
	int prediction;

	if (!model || !fv || model->hidden_dim > NN_MAX_HIDDEN)
		return ML_ACTION_ALLOW;

	/* Forward pass: input -> hidden */
	for (i = 0; i < model->hidden_dim; i++) {
		s64 sum = model->bias_hidden[i];
		for (j = 0; j < model->input_dim; j++)
			sum += (model->weights_input[i * model->input_dim + j] *
			        fv->features[j]) / FLOAT_SCALE;
		hidden[i] = relu(sum);
	}

	/* Forward pass: hidden -> output */
	for (i = 0; i < 3; i++) {
		s64 sum = model->bias_output[i];
		for (j = 0; j < model->hidden_dim; j++)
			sum += (model->weights_output[i * model->hidden_dim + j] *
			        hidden[j]) / FLOAT_SCALE;
		output[i] = sum;
	}

	/* Argmax (softmax approximation) */
	prediction = argmax(output, 3);

	return (enum ml_action)prediction;
}

int nn_model_load(struct nn_model *model, const void __user *data, size_t size)
{
	u32 header[4];
	size_t offset = 0;
	size_t expected_size;

	if (!model || !data || size < sizeof(header))
		return -EINVAL;

	if (copy_from_user(header, data, sizeof(header)))
		return -EFAULT;

	model->version = header[0];
	model->input_dim = header[1];
	model->hidden_dim = header[2];
	model->output_dim = header[3];

	if (model->input_dim != FEATURE_DIM || model->hidden_dim > NN_MAX_HIDDEN ||
	    model->output_dim != 3)
		return -EINVAL;

	offset = sizeof(header);
	expected_size = offset +
	                sizeof(s64) * model->input_dim * model->hidden_dim +  /* weights_input */
	                sizeof(s64) * model->hidden_dim +                     /* bias_hidden */
	                sizeof(s64) * model->hidden_dim * 3 +                 /* weights_output */
	                sizeof(s64) * 3;                                      /* bias_output */

	if (size < expected_size)
		return -EINVAL;

	/* Load input->hidden weights */
	if (copy_from_user(model->weights_input, (u8 __user *)data + offset,
	                   sizeof(s64) * model->input_dim * model->hidden_dim))
		return -EFAULT;
	offset += sizeof(s64) * model->input_dim * model->hidden_dim;

	/* Load hidden bias */
	if (copy_from_user(model->bias_hidden, (u8 __user *)data + offset,
	                   sizeof(s64) * model->hidden_dim))
		return -EFAULT;
	offset += sizeof(s64) * model->hidden_dim;

	/* Load hidden->output weights */
	if (copy_from_user(model->weights_output, (u8 __user *)data + offset,
	                   sizeof(s64) * model->hidden_dim * 3))
		return -EFAULT;
	offset += sizeof(s64) * model->hidden_dim * 3;

	/* Load output bias */
	if (copy_from_user(model->bias_output, (u8 __user *)data + offset,
	                   sizeof(s64) * 3))
		return -EFAULT;

	pr_info("kernel-ml: Loaded NN model v%u: %u -> %u -> %u\n",
	        model->version, model->input_dim, model->hidden_dim, model->output_dim);

	return 0;
}

void nn_model_free(struct nn_model *model)
{
	if (model)
		memset(model, 0, sizeof(*model));
}

/* ===== Unified Model Interface ===== */

enum ml_action unified_inference(struct unified_model *model, struct feature_vector *fv)
{
	if (!model || !fv)
		return ML_ACTION_ALLOW;

	switch (model->type) {
	case MODEL_TYPE_RANDOM_FOREST:
		return ml_inference(&model->data.rf, fv);
	case MODEL_TYPE_SVM:
		return svm_inference(&model->data.svm, fv);
	case MODEL_TYPE_LOGISTIC_REGRESSION:
		return lr_inference(&model->data.lr, fv);
	case MODEL_TYPE_NEURAL_NETWORK:
		return nn_inference(&model->data.nn, fv);
	default:
		return ML_ACTION_ALLOW;
	}
}

int unified_model_load(struct unified_model *model, enum model_type type,
                       const void __user *data, size_t size)
{
	if (!model)
		return -EINVAL;

	unified_model_free(model);
	model->type = type;

	switch (type) {
	case MODEL_TYPE_RANDOM_FOREST:
		return ml_model_load(&model->data.rf, data, size);
	case MODEL_TYPE_SVM:
		return svm_model_load(&model->data.svm, data, size);
	case MODEL_TYPE_LOGISTIC_REGRESSION:
		return lr_model_load(&model->data.lr, data, size);
	case MODEL_TYPE_NEURAL_NETWORK:
		return nn_model_load(&model->data.nn, data, size);
	default:
		return -EINVAL;
	}
}

void unified_model_free(struct unified_model *model)
{
	if (!model)
		return;

	switch (model->type) {
	case MODEL_TYPE_RANDOM_FOREST:
		ml_model_free(&model->data.rf);
		break;
	case MODEL_TYPE_SVM:
		svm_model_free(&model->data.svm);
		break;
	case MODEL_TYPE_LOGISTIC_REGRESSION:
		lr_model_free(&model->data.lr);
		break;
	case MODEL_TYPE_NEURAL_NETWORK:
		nn_model_free(&model->data.nn);
		break;
	}

	memset(model, 0, sizeof(*model));
}

EXPORT_SYMBOL_GPL(svm_inference);
EXPORT_SYMBOL_GPL(svm_model_load);
EXPORT_SYMBOL_GPL(lr_inference);
EXPORT_SYMBOL_GPL(lr_model_load);
EXPORT_SYMBOL_GPL(nn_inference);
EXPORT_SYMBOL_GPL(nn_model_load);
EXPORT_SYMBOL_GPL(unified_inference);
EXPORT_SYMBOL_GPL(unified_model_load);
EXPORT_SYMBOL_GPL(unified_model_free);
