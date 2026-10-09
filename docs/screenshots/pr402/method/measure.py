#!/usr/bin/env python3
# usage: measure.py BIN STATEDIR LABEL [profile-out.cpuprofile]
# Server for group t402 must be running on 6403 (seed.py started it).
# Loads the page, then sends 5 comment events and 3 two-file round events, one every 3 s,
# and reports long-task time per kind of event.
import json, os, subprocess, sys, time, urllib.request, random
B, ST, LABEL = sys.argv[1], sys.argv[2], sys.argv[3]
PROF = sys.argv[4] if len(sys.argv) > 4 else None
T = os.path.dirname(os.path.abspath(__file__))
env = dict(os.environ, XDG_STATE_HOME=ST)
AB = ['npx', '-y', 'agent-browser', '--session', 'pr-402']

def ab(*a, stdin=None):
    return subprocess.run(AB + list(a), capture_output=True, text=True, input=stdin, timeout=180).stdout.strip()

def sample():
    out = ab('eval', '--stdin', stdin=open(T + '/sample.js').read())
    return json.loads(json.loads(out))

def http(m, path, body=None):
    req = urllib.request.Request('http://localhost:6403' + path, data=json.dumps(body).encode() if body else None, method=m, headers={'Content-Type': 'application/json'})
    return json.loads(urllib.request.urlopen(req, timeout=30).read() or 'null')

st = http('GET', '/_/api/groups/t402')
maxround = 1000
ab('close')
ab('--init-script', T + '/init.js', 'open', 'http://localhost:6403/t402')
for _ in range(60):
    time.sleep(1)
    s = sample()
    if s.get('sidebarItems', 0) > 50 or _ > 30: break
time.sleep(5)
sample()  # drain the initial mount's long tasks
init = sample()
if PROF: ab('profiler', 'start')
random.seed(7)
res = {}
def phase(name, n, fn):
    sample()
    for i in range(n):
        fn(i)
        time.sleep(3)
    s = sample()
    res[name] = {'events': n, 'ltTotalMs': s['ltTotalMs'], 'ltCount': s['ltCount'], 'ltMaxMs': s['ltMaxMs'], 'perEventMs': round(s['ltTotalMs'] / n, 1)}
def comment(i):
    r = random.randint(1, 3); f = random.randint(2, 99)
    http('POST', '/_/api/groups/t402/comments', {'path': 'pkg%d/r%d_file%d.go' % (f % 10, r, f), 'side': 'new', 'startLine': 3, 'endLine': 3, 'body': 'event %d' % i, 'author': 'm'})
def rnd(i):
    d = subprocess.run(['python3', '-I', T + '/gen.py', str(maxround + i), '2', '0'], capture_output=True).stdout
    subprocess.run([B, '--port', '6403', '--target', 't402', '--no-open'], input=d, capture_output=True, env=env)
ONLY = os.environ.get('ONLY', '')
if ONLY != 'round': phase('comment', 5, comment)
if ONLY != 'comment': phase('round', 6 if ONLY == 'round' else 3, rnd)
if PROF: ab('profiler', 'stop', PROF)
fin = sample()
print(json.dumps({'label': LABEL, 'initNodes': init['nodes'], 'initHeapMB': init['heapMB'], 'sidebarItems': fin['sidebarItems'], 'sections': fin['sections'], 'nodes': fin['nodes'], 'heapMB': fin['heapMB'], **res}))
