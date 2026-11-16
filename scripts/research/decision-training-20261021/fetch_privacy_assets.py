"""Fetch only the pinned public PrivacyAlign/Mini assets for the bounded CPU run."""
import argparse
import hashlib
import os
from pathlib import Path
import tempfile
import urllib.request

from json_contract import read_json

ALIGN = 'https://huggingface.co/datasets/ServiceNow/PrivacyAlign/resolve/edaf507a2d1019b03b592b92a60d113b7eb46425/'
MINI = 'https://huggingface.co/google/bert_uncased_L-4_H-256_A-4/resolve/387825ce42dbb39b87911cdf8e383ee3b25184f8/'
TINY = 'https://huggingface.co/google/bert_uncased_L-2_H-128_A-2/resolve/30b0a37ccaaa32f332884b96992754e246e48c5f/'
ASSETS = (
    ('raw/PrivacyAlign_train.jsonl', ALIGN + 'data/train.jsonl'),
    ('raw/PrivacyAlign_test.jsonl', ALIGN + 'data/test.jsonl'),
    ('raw/PrivacyAlign_README.md', ALIGN + 'README.md'),
    ('base_model/vocab.txt', TINY + 'vocab.txt'),
    ('base_mini/config.json', MINI + 'config.json'),
    ('base_mini/pytorch_model.bin', MINI + 'pytorch_model.bin'),
    ('base_mini/README.md', MINI + 'README.md'),
)


def fetch_file(url, expected, path, *, opener=urllib.request.urlopen, max_bytes=96 << 20):
    if not isinstance(expected, str) or len(expected) != 64 or any(c not in '0123456789abcdef' for c in expected):
        raise ValueError('missing pinned asset SHA-256')
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        if path.is_symlink() or not path.is_file() or path.stat().st_size > max_bytes:
            raise ValueError('invalid existing asset')
        if hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            raise ValueError('existing asset digest mismatch')
        return path.stat().st_size
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as destination:
            temporary = Path(destination.name)
            digest, size = hashlib.sha256(), 0
            with opener(url, timeout=60) as response:
                while True:
                    chunk = response.read(1 << 20)
                    if not chunk:
                        break
                    size += len(chunk)
                    if size > max_bytes:
                        raise ValueError('asset byte budget exceeded')
                    digest.update(chunk)
                    destination.write(chunk)
        if digest.hexdigest() != expected:
            raise ValueError('downloaded asset digest mismatch')
        os.replace(temporary, path)
        return size
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assets', type=Path, required=True)
    args = parser.parse_args()
    hashes = read_json(Path(__file__).resolve().parents[1] / 'deep-optimization-20261018/source_hashes.json')
    if any(url not in hashes for _, url in ASSETS):
        raise ValueError('public asset pin missing')
    total = 0
    for relative, url in ASSETS:
        total += fetch_file(url, hashes[url], args.assets / relative)
        print(relative, hashes[url], flush=True)
    print('verified public bytes:', total, flush=True)


if __name__ == '__main__':
    main()
