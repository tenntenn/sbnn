#!/usr/bin/env python3
# usage: seed.py BIN STATEDIR NFILES NCOMMENTS  -> fresh server on 6403, group t402
import json, os, subprocess, sys, time, urllib.request, random
B, ST, N, NC = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4])
T = os.path.dirname(os.path.abspath(__file__))
env = dict(os.environ, XDG_STATE_HOME=ST)
subprocess.run([B, '--shutdown', '--port', '6403'], env=env, capture_output=True)
time.sleep(1)
subprocess.run(['rm', '-rf', ST]); os.makedirs(ST)
R = max(1, N // 100)
for r in range(1, R + 1):
    d = subprocess.run(['python3', '-I', T + '/gen.py', str(r), '100', '2'], capture_output=True).stdout
    p = subprocess.run([B, '--port', '6403', '--target', 't402', '--no-open'], input=d, capture_output=True, env=env)
    if p.returncode: print('round fail', r, p.stderr[:200])
def http(m, path, body=None):
    req = urllib.request.Request('http://localhost:6403' + path, data=json.dumps(body).encode() if body else None, method=m, headers={'Content-Type': 'application/json'})
    return json.loads(urllib.request.urlopen(req, timeout=30).read() or 'null')
random.seed(402)
for i in range(NC):
    r = random.randint(1, R); f = random.randint(2, 99)
    http('POST', '/_/api/groups/t402/comments', {'path': 'pkg%d/r%d_file%d.go' % (f % 10, r, f), 'side': 'new', 'startLine': 3, 'endLine': 3, 'body': 'comment %d' % i, 'author': 't'})
print('seeded', N, NC)
