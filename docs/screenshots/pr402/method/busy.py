#!/usr/bin/env python3
# usage: busy.py trace.json -> main-thread RunTask totals, and per-3s-bin busy ms (events are sent 3 s apart)
import json, sys, collections
tr = json.load(open(sys.argv[1]))['traceEvents']
xs = [e for e in tr if e.get('ph') == 'X' and 'dur' in e and e['name'] == 'RunTask']
cnt = collections.Counter((e['pid'], e['tid']) for e in xs if e['dur'] > 1000)
pt = max(cnt, key=cnt.get)
th = sorted((e for e in xs if (e['pid'], e['tid']) == pt), key=lambda e: e['ts'])
def tot(f): return sum(e['dur'] for e in th if f(e['dur'])) / 1000
print('tasks %d, total %.0f ms, >=5ms: %.0f ms (%d), >=50ms: %.0f ms (%d)' % (
    len(th), tot(lambda d: True), tot(lambda d: d >= 5000), sum(1 for e in th if e['dur'] >= 5000),
    tot(lambda d: d >= 50000), sum(1 for e in th if e['dur'] >= 50000)))
# the profiler start is the one task that contains CpuProfiler::StartProfiling
start = min((e['ts'] for e in tr if e['name'] == 'CpuProfiler::StartProfiling'), default=th[0]['ts'])
bins = collections.Counter()
for e in th:
    if e['ts'] < start + 300000: continue  # skip the profiler start itself
    bins[int((e['ts'] - start - 300000) // 3e6)] += e['dur']
print('per 3 s bin (ms):', ' '.join('%.0f' % (bins[i] / 1000) for i in range(max(bins) + 1)))
