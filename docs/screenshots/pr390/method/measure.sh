#!/bin/bash
# usage: measure.sh <label> : load the page fresh and probe initial mount
S=/tmp/claude-1000/p390
A="npx -y agent-browser --session pr-390"
$A close >/dev/null 2>&1
$A open --init-script $S/init.js "http://localhost:6399/t390" >/dev/null 2>&1
$A set viewport 1400 900 >/dev/null 2>&1
$A eval --stdin < $S/probe.js
