#!/bin/bash
# usage: ev.sh <script.js> : run a script in the pr-400 browser session's page
npx -y agent-browser --session pr-400 eval --stdin < "$1"
