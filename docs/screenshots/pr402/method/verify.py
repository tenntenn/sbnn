#!/usr/bin/env python3
# Behaviour check against the running server (port 6403, group t402): sidebar jump, active file,
# lazy mount, all after a round arrives. Prints one line per check.
import json, os, subprocess, sys, time
T = os.path.dirname(os.path.abspath(__file__))
B = sys.argv[1]
env = dict(os.environ, XDG_STATE_HOME=sys.argv[2])
AB = ['npx', '-y', 'agent-browser', '--session', 'pr-402-verify']

def ev(js):
    out = subprocess.run(AB + ['eval', '--stdin'], input=js, capture_output=True, text=True).stdout.strip()
    try: return json.loads(json.loads(out))
    except Exception: return out

subprocess.run(AB + ['close'], capture_output=True)
subprocess.run(AB + ['open', 'http://localhost:6403/t402'], capture_output=True)
time.sleep(8)
STATE = "JSON.stringify({active:(document.querySelector('.file-item.active .file-path')||{}).title, tables:document.querySelectorAll('.diff-stack table').length, items:document.querySelectorAll('.file-item').length, top:Math.round(document.querySelector('.split-pane').scrollTop)})"
print('start', ev(STATE))
CLICK = "(()=>{const items=[...document.querySelectorAll('.file-item')];const el=items[%d];el.click();return JSON.stringify(el.querySelector('.file-path').title)})()"
for idx in (1500, 40, 2900):
    clicked = ev(CLICK % idx)
    time.sleep(2.5)
    s = ev(STATE)
    print('jump', idx, 'clicked', clicked, '->', s, 'OK' if s['active'] == clicked else 'MISMATCH')
# a round arrives while the reader is far down
d = subprocess.run(['python3', '-I', T + '/gen.py', '777', '2', '0'], capture_output=True).stdout
subprocess.run([B, '--port', '6403', '--target', 't402', '--no-open'], input=d, capture_output=True, env=env)
time.sleep(3)
s = ev(STATE)
print('after round', s)
clicked = ev(CLICK % 0)
time.sleep(2.5)
s = ev(STATE)
print('jump first', clicked, s, 'OK' if s['active'] == clicked else 'MISMATCH')
# scroll to the very bottom: the new round's files must become active and mounted
ev("(()=>{const p=document.querySelector('.split-pane');p.scrollTop=p.scrollHeight;return '1'})()")
time.sleep(2.5)
print('bottom', ev(STATE))
