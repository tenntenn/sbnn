#!/bin/bash
# usage: run.sh <binary> <workdir> <label>
# Starts sbnn on the seeded fixture (setup.sh), loads the page fresh in
# headless Chrome, runs find.js, and prints server CPU ticks and RSS around it.
B=$1; S=$2; L=$3; P=6402; G=t400
HERE=$(dirname "$(readlink -f "$0")")
A="npx -y agent-browser --session pr-400"
export XDG_STATE_HOME=$S/state
$B --shutdown --port $P >/dev/null 2>&1
sleep 1
env -C "$S/fx" $B --port $P --target $G --no-open </dev/null >/dev/null 2>&1 &
sleep 2
PID=$(pgrep -f "sbnn-$L.*--port $P" | head -1)
[ -z "$PID" ] && PID=$(pgrep -f "$B" | head -1)
ticks() { awk '{print $14+$15}' /proc/$PID/stat; }
rss() { awk '/VmRSS|VmHWM/ {printf "%s=%s ", $1, $2}' /proc/$PID/status; }
echo "server pid $PID; before page load: ticks=$(ticks) $(rss)"
$A close >/dev/null 2>&1
$A open --init-script "$HERE/../../pr390/method/init.js" "http://localhost:$P/$G" >/dev/null 2>&1
$A set viewport 1400 900 >/dev/null 2>&1
sleep 3
T0=$(ticks)
echo "after load, before key: ticks=$T0 $(rss)"
$A eval --stdin < "$HERE/find.js"
echo "after find: ticks=$(ticks) (+$(( $(ticks) - T0 )) ticks, 100/s) $(rss)"
$A screenshot "$S/$L.png" >/dev/null 2>&1
