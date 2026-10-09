(async () => {
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const st = document.querySelector('.diff-stack');
  let sc = st; while (sc && !(sc.scrollHeight > sc.clientHeight + 5 && /auto|scroll/.test(getComputedStyle(sc).overflowY))) sc = sc.parentElement;
  const secs = () => [...st.querySelectorAll('.file-section')];
  const mountedN = () => secs().filter(s => s.children.length > 0).length;
  const nodes = () => document.getElementsByTagName('*').length;
  const out = {};
  await sleep(3000);
  out.before = { nodes: nodes(), mounted: mountedN(), sections: secs().length, scrollTop: Math.round(sc.scrollTop) };
  const firstSec = secs()[0];
  out.before.firstTop = Math.round(firstSec.getBoundingClientRect().top);
  window.getSelection().removeAllRanges();
  out.before.findFar = window.find('zzfarneedle398', true, false, true);
  window.getSelection().removeAllRanges();
  sc.scrollTop = 0; await sleep(200);

  // frame gaps and long tasks from the keydown on
  const gaps = []; let last = performance.now(); let run = true;
  (function tick() { const n = performance.now(); gaps.push(n - last); last = n; if (run) requestAnimationFrame(tick) })();
  // input latency probe: a timer that should fire every 10 ms
  const lag = []; let lt = performance.now();
  const iv = setInterval(() => { const n = performance.now(); lag.push(n - lt - 10); lt = n }, 10);
  const ltBefore = window.__lt.length;
  const t0 = performance.now();
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, bubbles: true, cancelable: true }));
  let done = null;
  while (performance.now() - t0 < 60000) {
    await sleep(25);
    if (mountedN() >= secs().length) { done = performance.now() - t0; break }
  }
  run = false; clearInterval(iv);
  const lts = window.__lt.slice(ltBefore);
  const sorted = [...gaps].sort((a, b) => a - b);
  out.hold = {
    msToAllMounted: done === null ? null : Math.round(done),
    nodes: nodes(), mounted: mountedN(),
    longtasks: lts.length, longtaskTotalMs: lts.reduce((a, x) => a + x[1], 0), longtaskMaxMs: lts.reduce((a, x) => Math.max(a, x[1]), 0),
    frames: gaps.length, frameGapMaxMs: Math.round(Math.max(...gaps)), frameGapP95Ms: Math.round(sorted[Math.floor(sorted.length * 0.95)]),
    timerLagMaxMs: Math.round(Math.max(...lag)), timerLagP95Ms: Math.round([...lag].sort((a, b) => a - b)[Math.floor(lag.length * 0.95)]),
    scrollTop: Math.round(sc.scrollTop), firstTop: Math.round(firstSec.getBoundingClientRect().top),
    heapMB: Math.round(performance.memory.usedJSHeapSize / 1048576),
  };
  window.getSelection().removeAllRanges();
  out.hold.findFar = window.find('zzfarneedle398', true, false, true);
  const sel = window.getSelection();
  out.hold.foundIn = sel.rangeCount ? (sel.getRangeAt(0).commonAncestorContainer.parentElement.closest('.file-section') || {}).id : null;
  out.hold.foundText = String(sel).slice(0, 40);
  await sleep(500);
  out.hold.afterFind = { scrollTop: Math.round(sc.scrollTop), nodes: nodes(), mounted: mountedN() };
  // release with Escape
  window.getSelection().removeAllRanges();
  sc.scrollTop = 0; await sleep(300);
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await sleep(1500);
  out.released = { nodes: nodes(), mounted: mountedN(), scrollTop: Math.round(sc.scrollTop), firstTop: Math.round(firstSec.getBoundingClientRect().top) };
  window.getSelection().removeAllRanges();
  out.released.findFarAgain = window.find('zzfarneedle398', true, false, true);
  window.getSelection().removeAllRanges();
  return JSON.stringify(out, null, 1);
})()
