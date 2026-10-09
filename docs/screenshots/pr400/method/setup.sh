#!/bin/bash
# usage: setup.sh <binary> <workdir>
# Seeds group t400 on port 6402: 20 rounds x 20 files (400 files, 40 of them
# 300-line Markdown). The Markdown files are also written to <workdir>/fx,
# and the server is started with `env -C <workdir>/fx` so previews read the
# working tree.
B=$1; S=$2; P=6402; G=t400
HERE=$(dirname "$(readlink -f "$0")")
export XDG_STATE_HOME=$S/state; rm -rf "$XDG_STATE_HOME" "$S/fx"; mkdir -p "$XDG_STATE_HOME" "$S/fx"
$B --shutdown --port $P >/dev/null 2>&1
for r in $(seq 1 20); do
  python3 -I "$HERE/gen.py" $r 20 2 "$S/fx" > "$S/r.diff"
  env -C "$S/fx" $B --port $P --target $G --no-open < "$S/r.diff" >/dev/null 2>&1 || echo "diff $r failed"
done
curl -s http://localhost:$P/_/api/groups/$G | head -c 200; echo
