#!/usr/bin/env python3
"""Default ETL transform: copies the input file's line count to stdout."""
import argparse
import sys

parser = argparse.ArgumentParser()
parser.add_argument("--input", required=True)
args = parser.parse_args()

try:
    with open(args.input) as fh:
        print(f"transformed {sum(1 for _ in fh)} lines from {args.input}")
except OSError as exc:
    print(f"transform failed: {exc}", file=sys.stderr)
    sys.exit(1)
