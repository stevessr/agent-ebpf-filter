#!/usr/bin/env python3
"""Unit checks for the kernel-ml binary UAPI formats."""

import struct
from pathlib import Path

import model_loader

FEATURE_DIM = model_loader.FEATURE_DIM


def test_tree_node_layout() -> None:
    assert model_loader.TREE_NODE_SIZE == 32
    packed = model_loader.pack_tree([
        {"feature_idx": 0, "threshold": 1000, "left_child": 1, "right_child": 2, "leaf_value": 0, "is_leaf": 0},
        {"feature_idx": 0, "threshold": 0, "left_child": -1, "right_child": -1, "leaf_value": 1, "is_leaf": 1},
    ])
    assert len(packed) == 4 + 2 * 32
    assert struct.unpack_from("<I", packed, 0)[0] == 2


def test_v2_header_layout() -> None:
    header = struct.pack("<IIIIII", 2, 7, FEATURE_DIM, 0, 4, 12)
    assert len(header) == 24
    assert struct.unpack("<IIIIII", header) == (2, 7, FEATURE_DIM, 0, 4, 12)


def test_cuda_abi_layouts() -> None:
    feature_vector_size = struct.calcsize("<128qII16s")
    request_size = struct.calcsize("<IIQQ") + feature_vector_size
    result_size = struct.calcsize("<IIQII")
    assert feature_vector_size == 1048
    assert request_size == 1072
    assert result_size == 24


def main() -> int:
    test_tree_node_layout()
    test_v2_header_layout()
    test_cuda_abi_layouts()
    print("kernel-ml model format tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
