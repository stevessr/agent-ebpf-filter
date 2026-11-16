"""Pinned downloads never install corrupt or partial assets."""
import hashlib
import io
import tempfile
import unittest
from pathlib import Path

from fetch_privacy_assets import fetch_file


class FetchAssetsTests(unittest.TestCase):
    def test_verified_install_and_digest_checked_reuse(self):
        raw = b'public fixture'
        digest = hashlib.sha256(raw).hexdigest()
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'asset'
            opener = lambda url, timeout: io.BytesIO(raw)
            self.assertEqual(fetch_file('https://example.invalid', digest, path, opener=opener), len(raw))
            def no_network(*args, **kwargs):
                raise AssertionError('cached verified asset should not download')
            self.assertEqual(fetch_file('https://example.invalid', digest, path, opener=no_network), len(raw))
            path.write_bytes(b'corrupt')
            with self.assertRaisesRegex(ValueError, 'digest mismatch'):
                fetch_file('https://example.invalid', digest, path, opener=no_network)

    def test_bad_digest_and_oversize_leave_no_partial_file(self):
        raw = b'public fixture'
        for digest, budget in (('a'*64, 100), (hashlib.sha256(raw).hexdigest(), 5)):
            with tempfile.TemporaryDirectory() as tmp:
                path = Path(tmp) / 'asset'
                with self.assertRaises(ValueError):
                    fetch_file('https://example.invalid', digest, path,
                               opener=lambda url, timeout: io.BytesIO(raw), max_bytes=budget)
                self.assertEqual(list(Path(tmp).iterdir()), [])


if __name__ == '__main__':
    unittest.main()
