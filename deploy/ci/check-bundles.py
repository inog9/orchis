#!/usr/bin/env python3
"""Check actual generated chunk sizes; never raise Vite's warning threshold."""
from pathlib import Path
import sys
files=list((Path(__file__).resolve().parents[2]/"web/dist/assets").glob("*.js"))
if not files:raise SystemExit("No generated JavaScript chunks; build first")
largest=max(files,key=lambda p:p.stat().st_size)
print(f"Largest JS chunk: {largest.name}: {largest.stat().st_size} bytes (limit 500000)")
if largest.stat().st_size>500000:sys.exit(1)
