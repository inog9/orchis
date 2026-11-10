import tempfile
import unittest
from pathlib import Path

from filter_http_templates import filter_templates


class FilterHTTPTemplatesTest(unittest.TestCase):
    def test_keeps_http_and_removes_mixed_and_unknown_protocols(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "http.yaml").write_text('id: http\nhttp: []\n')
            (root / "legacy.yml").write_text('id: legacy\nrequests: []\n')
            (root / "mixed.yaml").write_text('id: mixed\nhttp: []\n"tcp": []\n')
            (root / "future.yaml").write_text('id: future\nhttp: []\nnew-protocol: []\n')
            (root / "only-dns.yaml").write_text('id: dns\ndns: []\n')
            self.assertEqual(filter_templates(root), (2, 3))
            self.assertEqual(sorted(p.name for p in root.iterdir()), ['http.yaml', 'legacy.yml'])

    def test_invalid_yaml_does_not_delete_any_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "a-mixed.yaml").write_text('id: mixed\nhttp: []\ntcp: []\n')
            (root / "z-invalid.yaml").write_text('http: [\n')
            with self.assertRaises(Exception):
                filter_templates(root)
            self.assertTrue((root / "a-mixed.yaml").exists())

    def test_fails_closed_if_no_http_templates_remain(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "mixed.yaml").write_text('id: mixed\nhttp: []\ntcp: []\n')
            with self.assertRaisesRegex(ValueError, "no HTTP-only templates remain"):
                filter_templates(root)
            self.assertTrue((root / "mixed.yaml").exists())


if __name__ == "__main__":
    unittest.main()
