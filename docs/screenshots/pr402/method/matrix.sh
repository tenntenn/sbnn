#!/bin/bash
# before/after at 400 / 1500 / 3000 files, minified builds
T=/tmp/claude-1000/t402
OUT=$T/results.jsonl
: > $OUT
for rep in 1 2; do
for spec in "400 133" "1500 500" "3000 1000"; do
  set -- $spec
  for bin in base after; do
    python3 -I $T/seed.py $T/sbnn-$bin-min $T/state-m $1 $2 > /dev/null
    python3 -I $T/measure.py $T/sbnn-$bin-min $T/state-m "$bin-$1-r$rep" 2>&1 | tail -1 >> $OUT
  done
done
done
echo done >> $OUT
