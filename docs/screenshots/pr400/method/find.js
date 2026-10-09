(async () => {
  // usage: agent-browser eval --stdin < find.js (page opened with init.js from pr390/method)
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const pv = document.querySelector('.preview-stack');
  const secs = () => [...pv.querySelectorAll('.file-section')];
  const mountedN = () => secs().filter(s => s.children.length > 0 && !s.querySelector('.preview-body-unloaded')).length;
  const nodes = () => document.getElementsByTagName('*').length;
  const reqs = () => performance.getEntriesByType('resource').filter(e => /\/content/.test(e.name)).length;
  const mdLoaded = () => pv.querySelectorAll('.markdown').length;
  const find = (t) => { window.getSelection().removeAllRanges(); const r = window.find(t, true, false, true); const s = window.getSelection(); const where = s.rangeCount ? (s.getRangeAt(0).startContainer.parentElement.closest('.preview-stack') ? 'preview' : 'other') : null; window.getSelection().removeAllRanges(); return [r, where] };
  const out = {};
  await sleep(3000);
  out.before = { nodes: nodes(), previewRequests: reqs(), markdownMounted: mdLoaded(), findPreviewNeedle: find('zzmdneedle400'), findSource: find('zz&#109;dneedle400') };
  const gaps = []; let last = performance.now(); let run = true;
  (function tick() { const n = performance.now(); gaps.push(n - last); last = n; if (run) requestAnimationFrame(tick) })();
  const ltBefore = window.__lt.length;
  const t0 = performance.now();
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, bubbles: true, cancelable: true }));
  let doneMd = null;
  while (performance.now() - t0 < 20000) {
    await sleep(25);
    if (mdLoaded() >= 40) { doneMd = performance.now() - t0; break }
  }
  await sleep(500);
  run = false;
  const lts = window.__lt.slice(ltBefore);
  out.hold = {
    msAll40MarkdownMounted: doneMd === null ? null : Math.round(doneMd),
    nodes: nodes(), previewRequests: reqs(), markdownMounted: mdLoaded(), iframes: pv.querySelectorAll('iframe').length,
    longtasks: lts.length, longtaskTotalMs: lts.reduce((a, x) => a + x[1], 0), longtaskMaxMs: lts.reduce((a, x) => Math.max(a, x[1]), 0),
    frameGapMaxMs: Math.round(Math.max(...gaps)),
    findPreviewNeedle: find('zzmdneedle400'),
  };
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await sleep(1500);
  out.released = { nodes: nodes(), previewRequests: reqs(), markdownMounted: mdLoaded(), findPreviewNeedle: find('zzmdneedle400') };
  return JSON.stringify(out, null, 1);
})()
