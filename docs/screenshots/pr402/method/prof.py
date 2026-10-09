#!/usr/bin/env python3
# usage: prof.py trace.json [nself] [nincl] -> busy time, top self and top inclusive by function
import json, sys, collections
tr = json.load(open(sys.argv[1]))['traceEvents']
profs = collections.defaultdict(lambda: {'nodes': {}, 'samples': [], 'dt': []})
for e in tr:
    if e['name'] != 'ProfileChunk': continue
    d = e['args']['data']; cp = d['cpuProfile']
    P = profs[(e['pid'], e.get('id'))]
    for n in cp.get('nodes', []):
        P['nodes'][n['id']] = n
    P['samples'] += cp.get('samples', [])
    P['dt'] += d.get('timeDeltas', [])
key = max(profs, key=lambda k: len(profs[k]['samples']))
P = profs[key]
nodes = P['nodes']
parent = {}
for n in nodes.values():
    for c in n.get('children', []): parent[c] = n['id']
    if 'parent' in n: parent[n['id']] = n['parent']
def name(n):
    cf = n['callFrame']; return '%s %s:%d' % (cf['functionName'] or '(anon)', cf.get('url', '').split('/')[-1], cf.get('lineNumber', 0) + 1)
self_t = collections.Counter()
for s, d in zip(P['samples'], P['dt']): self_t[s] += d
total = sum(self_t.values())
idle = sum(t for i, t in self_t.items() if nodes[i]['callFrame']['functionName'] == '(idle)')
print('total %.0f ms, idle %.0f ms, busy %.0f ms' % (total / 1000, idle / 1000, (total - idle) / 1000))
by_self = collections.Counter(); incl = collections.Counter()
for i, t in self_t.items():
    n = nodes[i]
    if n['callFrame']['functionName'] == '(idle)': continue
    by_self[name(n)] += t
    seen = set(); j = i
    while j is not None:
        nm = name(nodes[j])
        if nm not in seen:
            incl[nm] += t; seen.add(nm)
        j = parent.get(j)
print('--- top self (ms)')
for k, v in by_self.most_common(int(sys.argv[2]) if len(sys.argv) > 2 else 25): print('%8.1f  %s' % (v / 1000, k))
print('--- top inclusive (ms)')
for k, v in incl.most_common(int(sys.argv[3]) if len(sys.argv) > 3 else 45): print('%8.1f  %s' % (v / 1000, k))
