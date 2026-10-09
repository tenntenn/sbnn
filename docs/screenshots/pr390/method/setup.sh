#!/bin/bash
# usage: setup.sh <binary> ; seeds group t390 on port 6399 with 400 files (40 md), 300 comments
S=/tmp/claude-1000/p390
B=$1; P=6399; G=t390
export XDG_STATE_HOME=$S/state; rm -rf "$XDG_STATE_HOME"; mkdir -p "$XDG_STATE_HOME"
$B --shutdown --port $P >/dev/null 2>&1
for r in $(seq 1 20); do python3 -I $S/gen.py $r 20 2 > $S/r.diff; $B --port $P --target $G --no-open < $S/r.diff >/dev/null 2>&1 || echo "diff $r failed"; done
for i in $(seq 1 300); do r=$(( (i-1)%20 )); f=$(( 2 + (i%18) ))
  $B comment --target $G --port $P --author t "pkg$((f%10))/r$((r+1))_file$f.go:3" -m "comment $i" >/dev/null 2>&1 || echo "comment $i failed"; done
curl -s http://localhost:$P/_/api/groups/$G | head -c 200; echo
