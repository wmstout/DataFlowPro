#!/usr/bin/env python3
"""Schema validator stub: checks the sample dataset header."""
import sys

EXPECTED = "id,name,owner,row_count,description"

with open("/data/datasets/sample.csv") as fh:
    header = fh.readline().strip()

if header != EXPECTED:
    print(f"schema mismatch: {header}")
    sys.exit(1)
print("schema ok")
