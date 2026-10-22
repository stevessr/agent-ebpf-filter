"""Ambiguous labels/tags and oversized review inputs must be rejected."""
import json
import tempfile
import unittest
from pathlib import Path

from json_contract import read_json, read_jsonl


class JSONContractTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name) / 'input.jsonl'

    def test_utf8_and_blank_lines_preserved(self):
        self.path.write_text('\n' + json.dumps(dict(label=0, tags=['文件读取']), ensure_ascii=False) + '\n\n')
        self.assertEqual(read_jsonl(self.path)[0]['tags'], ['文件读取'])

    def test_duplicate_labels_or_nested_tags_are_not_last_wins(self):
        for raw in ('{"label":1,"label":0}', '{"metadata":{"tag":"file_read","tag":"file_write"}}'):
            self.path.write_text(raw + '\n')
            for reader in (read_json, read_jsonl):
                with self.assertRaisesRegex(ValueError, 'duplicate'):
                    reader(self.path)

    def test_nonfinite_literals_and_exponent_overflow_rejected(self):
        for raw in ('NaN', 'Infinity', '-Infinity', '1e999'):
            self.path.write_text('{"score":' + raw + '}\n')
            with self.assertRaisesRegex(ValueError, 'non-finite'):
                read_jsonl(self.path)

    def test_nonobject_and_invalid_utf8_rejected(self):
        for raw in (b'[]\n', b'null\n', b'1\n', b'{"tag":"\xff"}\n'):
            self.path.write_bytes(raw)
            with self.assertRaises(ValueError):
                read_jsonl(self.path)

    def test_line_total_and_row_budgets(self):
        self.path.write_text('{"label":0}\n' * 2)
        for limits in (dict(max_line_bytes=5), dict(max_bytes=15), dict(max_rows=1)):
            with self.assertRaisesRegex(ValueError, 'budget'):
                read_jsonl(self.path, **limits)
        with self.assertRaisesRegex(ValueError, 'budget'):
            read_json(self.path, max_bytes=5)

    def test_budgets_are_bytes_and_allow_exact_boundary(self):
        raw = '{"tag":"文件读取"}\n'.encode()
        self.path.write_bytes(raw)
        self.assertEqual(len(read_jsonl(self.path, max_line_bytes=len(raw), max_bytes=len(raw))), 1)
        with self.assertRaisesRegex(ValueError, 'budget'):
            read_jsonl(self.path, max_line_bytes=len(raw)-1)


if __name__ == '__main__':
    unittest.main()
