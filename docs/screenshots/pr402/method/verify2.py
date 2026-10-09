#!/usr/bin/env python3
# The active file's round is removed while the reader is on it: the sidebar highlight must move on.
import json, subprocess, time, urllib.request
AB = ['npx', '-y', 'agent-browser', '--session', 'pr-402-verify']
def ev(js):
    out = subprocess.run(AB + ['eval', '--stdin'], input=js, capture_output=True, text=True).stdout.strip()
    try: return json.loads(json.loads(out))
    except Exception: return out
def http(m, p):
    return json.loads(urllib.request.urlopen(urllib.request.Request('http://localhost:6403' + p, method=m), timeout=30).read() or 'null')
subprocess.run(AB + ['close'], capture_output=True)
subprocess.run(AB + ['open', 'http://localhost:6403/t402'], capture_output=True)
time.sleep(8)
STATE = "JSON.stringify({active:(document.querySelector('.file-item.active .file-path')||{}).title, items:document.querySelectorAll('.file-item').length})"
items = http('GET', '/_/api/groups/t402')['diffs']
last = items[-1]
print('last round', last['id'], last['title'], len(last['files']))
ev("(()=>{const it=[...document.querySelectorAll('.file-item')].slice(-1)[0];it.click();return '1'})()")
time.sleep(2.5)
print('before delete', ev(STATE))
http('DELETE', '/_/api/groups/t402/diffs/' + last['id'])
time.sleep(3)
print('after delete', ev(STATE))
