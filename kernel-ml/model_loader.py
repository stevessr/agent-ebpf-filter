#!/usr/bin/env python3
"""
Model Loader - Convert trained Random Forest to kernel format

This script takes a trained RandomForest model and converts it to
the binary format expected by the kernel module.

Format:
- Header: [version:u32, num_trees:u32, feature_dim:u32, total_nodes:u32]
- For each tree:
  - [num_nodes:u32]
  - Array of tree_node structs (32 bytes each)
"""

import struct
import sys
import pickle

FEATURE_DIM = 128
FLOAT_SCALE = 1000
TREE_NODE_FORMAT = '<IIqiiiB3x'
TREE_NODE_SIZE = struct.calcsize(TREE_NODE_FORMAT)
assert TREE_NODE_SIZE == 32

def float_to_fixed(f):
    return int(f * FLOAT_SCALE)

def export_tree_nodes(tree):
    """Convert sklearn tree to flat node array"""
    nodes = []

    def traverse(idx):
        if idx < 0 or idx >= len(tree.feature):
            return

        feature_idx = tree.feature[idx]
        threshold = tree.threshold[idx]
        left_child = tree.children_left[idx]
        right_child = tree.children_right[idx]

        is_leaf = (left_child == right_child == -1)

        if is_leaf:
            # Leaf node: value is class label
            value = tree.value[idx]
            leaf_value = value.argmax()  # ALLOW=0, BLOCK=1, ALERT=2
        else:
            leaf_value = 0

        node = {
            'feature_idx': int(feature_idx) if not is_leaf else 0,
            'threshold': float_to_fixed(threshold) if not is_leaf else 0,
            'left_child': int(left_child),
            'right_child': int(right_child),
            'leaf_value': int(leaf_value),
            'is_leaf': 1 if is_leaf else 0,
        }

        nodes.append(node)

        if not is_leaf:
            traverse(left_child)
            traverse(right_child)

    traverse(0)
    return nodes

def pack_tree(nodes):
    """Pack nodes into binary format"""
    data = struct.pack('I', len(nodes))

    for node in nodes:
        # struct tree_node: fixed 32-byte little-endian UAPI layout:
        # feature_idx, pad0, threshold, left, right, leaf_value, is_leaf, pad[3]
        packed = struct.pack(
            TREE_NODE_FORMAT,
            node['feature_idx'],
            0,
            node['threshold'],
            node['left_child'],
            node['right_child'],
            node['leaf_value'],
            node['is_leaf']
        )
        data += packed

    return data

def export_model(model, output_path):
    """Export RandomForest to kernel binary format"""

    num_trees = len(model.estimators_)
    feature_dim = FEATURE_DIM

    # Header
    data = struct.pack('IIII', 1, num_trees, feature_dim, 0)  # version=1, total_nodes=0 (unused)

    # Each tree
    for tree_estimator in model.estimators_:
        tree = tree_estimator.tree_
        nodes = export_tree_nodes(tree)
        data += pack_tree(nodes)

    with open(output_path, 'wb') as f:
        f.write(data)

    print(f"Exported model: {num_trees} trees, {feature_dim} features -> {output_path}")
    print(f"Binary size: {len(data)} bytes")

if __name__ == '__main__':
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <model.pkl> <output.bin>")
        sys.exit(1)

    with open(sys.argv[1], 'rb') as f:
        model = pickle.load(f)

    export_model(model, sys.argv[2])
