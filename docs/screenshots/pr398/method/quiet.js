window.__q = null;
(async () => {
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const st = document.querySelector('.diff-stack');
  const secs = () => [...st.querySelectorAll('.file-section')];
  const mountedN = () => secs().filter(s => s.children.length > 0).length;
  const out = {};
  await sleep(1000);
  const t0 = performance.now();
  const at = () => Math.round((performance.now() - t0) / 1000);
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, bubbles: true }));
  await sleep(6000);
  out['t=' + at() + 's after ctrl+f'] = mountedN();
  await sleep(34000);
  document.dispatchEvent(new MouseEvent('mousemove', { bubbles: true }));
  out['t=' + at() + 's mousemove sent'] = mountedN();
  await sleep(35000);
  out['t=' + at() + 's (35s after mousemove, 75s after ctrl+f)'] = mountedN();
  await sleep(30000);
  out['t=' + at() + 's (65s after mousemove)'] = mountedN();
  out.nodesAtEnd = document.getElementsByTagName('*').length;
  window.__q = JSON.stringify(out, null, 1);
})(); 'started'
