import sys
from pathlib import Path
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from service import choose_backend, utf16_offsets, collect_entities, merge_windows


class ServiceTests(unittest.TestCase):
    def test_backend_prefers_cuda_only_with_complete_model(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            (root / 'fp16').mkdir()
            for name in ('model_fp16_static512.onnx', 'labels.json', 'tokenizer.json'):
                (root / 'fp16' / name).touch()
            self.assertEqual(choose_backend('auto', ['CUDAExecutionProvider', 'CPUExecutionProvider'], root)[0], 'cuda')
            with self.assertRaises(RuntimeError):
                choose_backend('vulkan', ['CPUExecutionProvider'], root, strict=True)

    def test_unicode_offsets(self):
        self.assertEqual(utf16_offsets('😀你好a'), [0, 2, 3, 4, 5])

    def test_bio_decode(self):
        spans = collect_entities(['O', 'B-private_person', 'I-private_person', 'O'],
                                 [(0, 0), (1, 4), (5, 8), (0, 0)],
                                 [1, 0, 0, 1], [1, .95, .96, 1], .8)
        self.assertEqual(spans[0]['start'], 1)
        self.assertEqual(spans[0]['end'], 8)
        self.assertEqual(spans[0]['label'], 'private_person')

    def test_overlapping_windows(self):
        x = merge_windows([{'start': 1,'end': 8,'label':'email','score':.92},
                           {'start': 1,'end': 8,'label':'email','score':.99}])
        self.assertEqual(len(x), 1)
        self.assertEqual(x[0]['score'], .99)

if __name__ == '__main__':
    unittest.main()
