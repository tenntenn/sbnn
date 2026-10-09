#!/bin/bash
S=/tmp/claude-1000/-home-tenntenn-repo-tenntenn-sbnn--git-wt-wt-1/5b9c51bd-a7fe-493d-8986-196a238b5224/scratchpad
B=${1:-$S/sbnn}; P=6401; G=t398
export XDG_STATE_HOME=$S/state; rm -rf "$XDG_STATE_HOME"; mkdir -p "$XDG_STATE_HOME"
$B --shutdown --port $P >/dev/null 2>&1
for r in $(seq 1 20); do python3 -I $S/m/gen.py $r 20 2 > $S/r.diff; $B --port $P --target $G --no-open < $S/r.diff >/dev/null 2>&1 || echo "diff $r failed"; done
for i in $(seq 1 300); do r=$(( (i-1)%20 )); f=$(( 2 + (i%18) ))
  $B comment --target $G --port $P --author t "pkg$((f%10))/r$((r+1))_file$f.go:3" -m "comment $i" >/dev/null 2>&1 || echo "comment $i failed"; done
curl -s http://localhost:$P/_/api/groups/$G | head -c 200; echo
