#!/usr/bin/env python3
# usage: tasks.py trace.json [min_ms] -> main-thread tasks longer than min_ms with their biggest children (by name)
import json, sys, collections
tr = json.load(open(sys.argv[1]))['traceEvents']
mn = float(sys.argv[2]) if len(sys.argv) > 2 else 30
# main thread = tid with most RunTask events whose dur is large
xs = [e for e in tr if e.get('ph') == 'X' and 'dur' in e]
runs = [e for e in xs if e['name'] == 'RunTask']
cnt = collections.Counter((e['pid'], e['tid']) for e in runs if e['dur'] > 1000)
pt = max(cnt, key=cnt.get)
th = [e for e in xs if (e['pid'], e['tid']) == pt]
th.sort(key=lambda e: (e['ts'], -e['dur']))
big = [e for e in th if e['name'] == 'RunTask' and e['dur'] >= mn * 1000]
for t in big:
    kids = collections.Counter()
    for e in th:
        if e is t: continue
        if e['ts'] >= t['ts'] and e['ts'] + e['dur'] <= t['ts'] + t['dur'] and e['name'] not in ('RunTask', 'ThreadControllerImpl::RunTask'):
            kids[e['name']] += e['dur']
    top = ', '.join('%s %.0f' % (k, v / 1000) for k, v in kids.most_common(9))
    print('%7.1f ms @%.1fs  %s' % (t['dur'] / 1000, t['ts'] / 1e6 % 1000, top))
