#!/usr/bin/env python3
"""Exercise kernel_ml_cuda_helper request/result ABI without loading the DKMS module."""

from __future__ import annotations

import os
import struct
import subprocess
import sys
import tempfile
from pathlib import Path

FEATURE_DIM = 128
FLOAT_SCALE = 1000
ML_CUDA_REQUEST_VERSION = 1
ML_ACTION_BLOCK = 1
EXPECTED_CLASS = 3

TREE_NODE = "<IIqiiiB3x"
FEATURE_VECTOR = "<128qII16s"
REQUEST_PREFIX = "<IIQQ"
RESULT = "<IIQII"


def node(feature_idx: int, threshold: int, left: int, right: int, leaf: int, is_leaf: int) -> bytes:
    return struct.pack(TREE_NODE, feature_idx, 0, threshold, left, right, leaf, is_leaf)


def write_model(path: Path) -> None:
    # v2 model: version, num_trees, feature_dim, total_nodes, num_classes, max_depth
    data = bytearray(struct.pack("<IIIIII", 2, 1, FEATURE_DIM, 3, 4, 2))
    data += struct.pack("<I", 3)
    data += node(0, FLOAT_SCALE, 1, 2, 0, 0)
    data += node(0, 0, -1, -1, ML_ACTION_BLOCK, 1)
    data += node(0, 0, -1, -1, EXPECTED_CLASS, 1)
    path.write_bytes(data)


def write_request(path: Path) -> None:
    features = [0] * FEATURE_DIM
    features[0] = 2 * FLOAT_SCALE
    fv = struct.pack(FEATURE_VECTOR, *features, 4242, 257, b"cuda-proto\0")
    req = struct.pack(REQUEST_PREFIX, ML_CUDA_REQUEST_VERSION, 0, 42, 0) + fv
    assert len(req) == 1072
    path.write_bytes(req)


def main(argv: list[str]) -> int:
    helper = Path(argv[1] if len(argv) > 1 else "./kernel_ml_cuda_helper")
    if not helper.is_absolute():
        helper = helper.resolve()
    if not helper.exists():
        print(f"SKIP: helper not found: {helper}")
        return 0

    with tempfile.TemporaryDirectory(prefix="kernel-ml-cuda-proto-") as tmp_raw:
        tmp = Path(tmp_raw)
        model = tmp / "model.bin"
        request = tmp / "request.bin"
        result = tmp / "result.bin"
        write_model(model)
        write_request(request)
        result.write_bytes(b"")

        proc = subprocess.run(
            [str(helper), "--model", str(model), "--request", str(request), "--result", str(result), "--oneshot"],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=20,
            check=False,
        )
        combined = proc.stdout + proc.stderr
        if proc.returncode != 0:
            if "no CUDA device available" in combined:
                print("SKIP: no CUDA device available")
                return 0
            print(combined, file=sys.stderr)
            return proc.returncode

        raw = result.read_bytes()
        if len(raw) < struct.calcsize(RESULT):
            print(f"result too small: {len(raw)} bytes", file=sys.stderr)
            print(combined, file=sys.stderr)
            return 1
        version, status, request_id, action, _ = struct.unpack(RESULT, raw[: struct.calcsize(RESULT)])
        assert version == ML_CUDA_REQUEST_VERSION
        assert status == 0
        assert request_id == 42
        assert action == EXPECTED_CLASS, action
        print("kernel-ml CUDA helper protocol test passed")
        return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
