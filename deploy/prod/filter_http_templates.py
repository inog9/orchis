#!/usr/bin/env python3
"""Keep only Nuclei templates declaring HTTP requests and no other protocols."""
from pathlib import Path
import sys

import yaml

# Unknown top-level keys are rejected as well, so newly introduced protocols
# cannot silently enter this pinned template set.
HTTP_KEYS = {"http", "requests"}
SAFE_KEYS = HTTP_KEYS | {
    "id", "info", "flow", "variables", "constants", "self-contained",
    "stop-at-first-match", "signature",
}


def filter_templates(root):
    root = Path(root)
    if not root.is_dir() or root.is_symlink():
        raise ValueError("template root must be a directory")
    templates = sorted(p for p in root.rglob("*") if p.suffix.lower() in {".yaml", ".yml"})
    if not templates:
        raise ValueError("no templates found")
    rejected = []
    loader = getattr(yaml, "CSafeLoader", yaml.SafeLoader)
    for path in templates:
        if path.is_symlink() or not path.is_file():
            raise ValueError(f"invalid template: {path}")
        with path.open("rb") as source:
            document = yaml.load(source, Loader=loader)
        if not isinstance(document, dict) or not all(isinstance(k, str) for k in document):
            raise ValueError(f"invalid template mapping: {path}")
        keys = set(document)
        if not (keys & HTTP_KEYS) or keys - SAFE_KEYS:
            rejected.append(path)
    if len(rejected) == len(templates):
        raise ValueError("no HTTP-only templates remain")
    for path in rejected:
        path.unlink()
    return len(templates) - len(rejected), len(rejected)


if __name__ == "__main__":
    kept, removed = filter_templates(sys.argv[1])
    print(f"HTTP-only templates: {kept} kept, {removed} excluded")
