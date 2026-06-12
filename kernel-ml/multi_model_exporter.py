#!/usr/bin/env python3
"""
Multi-Model Exporter - Convert sklearn/PyTorch models to kernel format

Supports:
- RandomForest (sklearn)
- SVM (sklearn)
- LogisticRegression (sklearn)
- Simple MLP (PyTorch/sklearn)
"""

import struct
import sys
import pickle
import numpy as np

FEATURE_DIM = 128
FLOAT_SCALE = 1000

def float_to_fixed(f):
    return int(f * FLOAT_SCALE)

# ===== SVM Export =====
def export_svm(model, output_path):
    """Export sklearn SVM to kernel format"""

    # Header: [version, num_support_vectors, feature_dim, reserved]
    version = 1
    feature_dim = FEATURE_DIM

    # Linear SVM weights and bias
    weights = model.coef_[0] if hasattr(model, 'coef_') else np.zeros(FEATURE_DIM)
    bias = float(model.intercept_[0]) if hasattr(model, 'intercept_') else 0.0

    # Convert to fixed-point
    weights_fixed = [float_to_fixed(w) for w in weights[:FEATURE_DIM]]
    bias_fixed = float_to_fixed(bias)

    # Pack binary
    data = struct.pack('IIII', version, 0, feature_dim, 0)  # num_support_vectors=0 for linear
    data += struct.pack('q', bias_fixed)
    data += struct.pack('q' * FEATURE_DIM, *weights_fixed)

    with open(output_path, 'wb') as f:
        f.write(data)

    print(f"Exported SVM: {feature_dim} features -> {output_path}")

# ===== Logistic Regression Export =====
def export_lr(model, output_path):
    """Export sklearn LogisticRegression to kernel format"""

    version = 1
    feature_dim = FEATURE_DIM

    weights = model.coef_[0][:FEATURE_DIM]
    bias = float(model.intercept_[0])

    # Thresholds for 3-class (BLOCK/ALERT/ALLOW)
    threshold_low = 0.3  # < 0.3 = BLOCK
    threshold_high = 0.7  # > 0.7 = ALLOW, between = ALERT

    weights_fixed = [float_to_fixed(w) for w in weights]
    bias_fixed = float_to_fixed(bias)
    thresholds_fixed = [float_to_fixed(threshold_low), float_to_fixed(threshold_high)]

    # Pack binary
    data = struct.pack('II', version, feature_dim)
    data += struct.pack('q' * FEATURE_DIM, *weights_fixed)
    data += struct.pack('q', bias_fixed)
    data += struct.pack('qq', *thresholds_fixed)

    with open(output_path, 'wb') as f:
        f.write(data)

    print(f"Exported LR: {feature_dim} features -> {output_path}")

# ===== Neural Network Export =====
def export_nn(model, output_path, hidden_dim=32):
    """Export simple MLP to kernel format"""

    version = 1
    input_dim = FEATURE_DIM
    output_dim = 3

    # Extract weights (assume model has .get_weights() or similar)
    if hasattr(model, 'get_weights'):
        # Keras/TensorFlow
        weights = model.get_weights()
        W1, b1, W2, b2 = weights[0], weights[1], weights[2], weights[3]
    elif hasattr(model, 'coefs_'):
        # sklearn MLPClassifier
        W1, W2 = model.coefs_[0], model.coefs_[1]
        b1, b2 = model.intercepts_[0], model.intercepts_[1]
    else:
        raise ValueError("Model format not recognized")

    # Convert to fixed-point
    W1_fixed = [float_to_fixed(w) for w in W1.flatten()]
    b1_fixed = [float_to_fixed(b) for b in b1]
    W2_fixed = [float_to_fixed(w) for w in W2.flatten()]
    b2_fixed = [float_to_fixed(b) for b in b2]

    # Pack binary
    data = struct.pack('IIII', version, input_dim, hidden_dim, output_dim)
    data += struct.pack('q' * len(W1_fixed), *W1_fixed)
    data += struct.pack('q' * len(b1_fixed), *b1_fixed)
    data += struct.pack('q' * len(W2_fixed), *W2_fixed)
    data += struct.pack('q' * len(b2_fixed), *b2_fixed)

    with open(output_path, 'wb') as f:
        f.write(data)

    print(f"Exported NN: {input_dim} -> {hidden_dim} -> {output_dim} -> {output_path}")

# ===== CLI =====
def main():
    if len(sys.argv) < 4:
        print(f"Usage: {sys.argv[0]} <model_type> <model.pkl> <output.bin>")
        print("Model types: rf, svm, lr, nn")
        sys.exit(1)

    model_type = sys.argv[1]
    model_path = sys.argv[2]
    output_path = sys.argv[3]

    with open(model_path, 'rb') as f:
        model = pickle.load(f)

    if model_type == 'rf':
        from model_loader import export_model
        export_model(model, output_path)
    elif model_type == 'svm':
        export_svm(model, output_path)
    elif model_type == 'lr':
        export_lr(model, output_path)
    elif model_type == 'nn':
        hidden_dim = int(sys.argv[4]) if len(sys.argv) > 4 else 32
        export_nn(model, output_path, hidden_dim)
    else:
        print(f"Unknown model type: {model_type}")
        sys.exit(1)

if __name__ == '__main__':
    main()
