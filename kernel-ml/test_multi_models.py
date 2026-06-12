#!/usr/bin/env python3
"""
Test Multi-Model Support

Generates synthetic models and tests kernel module inference.
"""

import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.svm import LinearSVC
from sklearn.linear_model import LogisticRegression
from sklearn.neural_network import MLPClassifier
import pickle

# Generate synthetic training data
np.random.seed(42)
X_train = np.random.randn(1000, 128)
y_train = np.random.choice([0, 1, 2], size=1000)  # ALLOW, BLOCK, ALERT

print("=== Training Models ===\n")

# 1. Random Forest
print("1. Training RandomForest...")
rf = RandomForestClassifier(n_estimators=15, max_depth=7, random_state=42)
rf.fit(X_train, y_train)
with open('model_rf.pkl', 'wb') as f:
    pickle.dump(rf, f)
print(f"   Accuracy: {rf.score(X_train, y_train):.2f}")

# 2. SVM
print("\n2. Training SVM...")
svm = LinearSVC(random_state=42, max_iter=1000)
svm.fit(X_train, y_train)
with open('model_svm.pkl', 'wb') as f:
    pickle.dump(svm, f)
print(f"   Accuracy: {svm.score(X_train, y_train):.2f}")

# 3. Logistic Regression
print("\n3. Training LogisticRegression...")
lr = LogisticRegression(random_state=42, max_iter=1000)
lr.fit(X_train, y_train)
with open('model_lr.pkl', 'wb') as f:
    pickle.dump(lr, f)
print(f"   Accuracy: {lr.score(X_train, y_train):.2f}")

# 4. Neural Network
print("\n4. Training MLP...")
nn = MLPClassifier(hidden_layer_sizes=(32,), activation='relu',
                   max_iter=500, random_state=42)
nn.fit(X_train, y_train)
with open('model_nn.pkl', 'wb') as f:
    pickle.dump(nn, f)
print(f"   Accuracy: {nn.score(X_train, y_train):.2f}")

print("\n=== Exporting Models to Kernel Format ===\n")

import subprocess

# Export each model
models = [
    ('rf', 'model_rf.pkl', 'model_rf.bin'),
    ('svm', 'model_svm.pkl', 'model_svm.bin'),
    ('lr', 'model_lr.pkl', 'model_lr.bin'),
    ('nn', 'model_nn.pkl', 'model_nn.bin', '32'),
]

for model_spec in models:
    model_type = model_spec[0]
    pkl_path = model_spec[1]
    bin_path = model_spec[2]

    cmd = ['python3', 'multi_model_exporter.py', model_type, pkl_path, bin_path]
    if len(model_spec) > 3:
        cmd.append(model_spec[3])

    subprocess.run(cmd, check=True)

print("\n=== Model Files Ready ===")
print("Load into kernel with:")
print("  cat model_rf.bin > /proc/ml_load    # RandomForest")
print("  cat model_svm.bin > /proc/ml_load   # SVM")
print("  cat model_lr.bin > /proc/ml_load    # LogisticRegression")
print("  cat model_nn.bin > /proc/ml_load    # Neural Network")
