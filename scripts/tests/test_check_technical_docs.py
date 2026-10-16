#!/usr/bin/env python3
"""Regression tests for the code-anchored technical manual guard."""
from __future__ import annotations

import importlib.util
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "check_technical_docs", ROOT / "scripts" / "check-technical-docs.py"
)
assert SPEC is not None and SPEC.loader is not None
CHECK = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECK)


class TechnicalDocsValidationTests(unittest.TestCase):
    @staticmethod
    def sample_page() -> str:
        tick = chr(96) * 3
        anchor = chr(96) + "backend/main.go" + chr(96)
        diagram = f"{tick}mermaid\nflowchart LR\n  A --> B\n{tick}"
        return f"# Example\n\n{anchor}\n\n{diagram}\n\n{diagram}\n"

    def check_page(
        self, body: str | None = None, nav: str | None = None, index: str | None = None
    ) -> tuple[int, list[str]]:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "docs" / "technical").mkdir(parents=True)
            (root / "backend").mkdir()
            (root / "backend" / "main.go").write_text("package main\n", encoding="utf-8")
            page = root / "docs" / "technical" / "example.md"
            page.write_text(body if body is not None else self.sample_page(), encoding="utf-8")
            return CHECK.check_document(
                page,
                nav if nav is not None else "- Example: technical/example.md\n",
                index if index is not None else "[Example](./example.md)\n",
                root,
            )

    def test_valid_two_mermaid_blocks(self) -> None:
        self.assertEqual(self.check_page(), (2, []))

    def test_short_closing_fence_does_not_count(self) -> None:
        tick = chr(96) * 3
        broken = self.sample_page().replace(
            f"{tick}mermaid\nflowchart LR\n  A --> B\n{tick}",
            "~~~~mermaid\nflowchart LR\n  A --> B\n~~~",
            1,
        )
        count, issues = self.check_page(broken)
        self.assertEqual(count, 1)
        self.assertTrue(any("unclosed" in issue for issue in issues))

    def test_tilde_fences_are_supported(self) -> None:
        tick = chr(96) * 3
        body = self.sample_page().replace(tick, "~~~")
        self.assertEqual(self.check_page(body), (2, []))

    def test_parent_relative_missing_link_is_rejected(self) -> None:
        _, issues = self.check_page(self.sample_page() + "[Missing](../unknown.md)\n")
        self.assertTrue(any("broken" in issue for issue in issues))

    def test_source_anchor_must_exist(self) -> None:
        _, issues = self.check_page(self.sample_page().replace("backend/main.go", "backend/missing.go"))
        self.assertTrue(any("source path not found" in issue for issue in issues))
        self.assertTrue(any("missing resolvable" in issue for issue in issues))

    def test_nav_and_index_require_unique_entry(self) -> None:
        _, nav_issues = self.check_page(nav="- One: technical/example.md\n- Two: technical/example.md\n")
        self.assertTrue(any("MkDocs nav" in issue for issue in nav_issues))
        _, index_issues = self.check_page(index="[A](./example.md)\n[B](./example.md)\n")
        self.assertTrue(any("technical index link" in issue for issue in index_issues))


if __name__ == "__main__":
    unittest.main()
