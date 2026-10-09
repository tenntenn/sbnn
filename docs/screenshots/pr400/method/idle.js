(async () => {
  // usage: idle.js <phase>; run "start" once, then "check" after 70 s with no input.
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const pv = document.querySelector('.preview-stack');
  const nodes = () => document.getElementsByTagName('*').length;
  const md = () => pv.querySelectorAll('.markdown').length;
  if (!window.__idle) {
    window.__idle = { t0: performance.now(), nodes0: nodes() };
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, bubbles: true }));
    await sleep(9000);
    const t0 = window.__idle.t0;
    const es = performance.getEntriesByType('resource').filter(e => /\/content/.test(e.name) && e.startTime >= t0);
    const ev = [];
    for (const e of es) { ev.push([e.startTime, 1]); ev.push([e.responseEnd, -1]) }
    ev.sort((a, b) => a[0] - b[0] || a[1] - b[1]);
    let cur = 0, max = 0;
    for (const [, d] of ev) { cur += d; max = Math.max(max, cur) }
    return JSON.stringify({ afterKey9s: { nodes: nodes(), markdown: md(), contentRequestsSinceKey: es.length, maxConcurrentContentRequests: max } });
  }
  return JSON.stringify({ idleCheck: { sinceKeySec: Math.round((performance.now() - window.__idle.t0) / 1000), nodes: nodes(), markdown: md() } });
})()
