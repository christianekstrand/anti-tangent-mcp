#!/usr/bin/env bash
# check-tree.sh NN — run from the repository root. Compares the index, leaving
# out this plan's own files, with the tree the dry run had after task NN.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
want=$(grep "^$1 " "$here/TREES" | cut -d' ' -f3)
got=$(git ls-files -s -- . ':(exclude)docs/superpowers/plans/2026-10-04-field-data-part3-gate-cost*' | sha256sum | cut -d' ' -f1)
if [ -n "$want" ] && [ "$want" = "$got" ]; then
  echo "tree OK ($1)"
else
  echo "TREE MISMATCH for task $1: index $got, expected ${want:-<no such task>}"
  exit 1
fi
