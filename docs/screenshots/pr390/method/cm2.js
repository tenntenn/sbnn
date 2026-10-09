window.__res = null;
(async () => {
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const b = document.querySelector('.preview-toolbar button.active'); if (b) b.click();
  const pv = document.querySelector('.preview-stack');
  let sc = pv; while (sc && !(sc.scrollHeight > sc.clientHeight + 5 && /auto|scroll/.test(getComputedStyle(sc).overflowY))) sc = sc.parentElement;
  const target = [...pv.querySelectorAll('.file-section')].find(s => /r10\/doc1\.md/.test((s.querySelector('.preview-header .path') || {}).textContent || ''));
  const go = () => { sc.scrollTop += target.getBoundingClientRect().top - sc.getBoundingClientRect().top - 50; };
  go(); await sleep(1500); go(); await sleep(1000);
  sc.scrollTop = 0; await sleep(1500); go(); await sleep(1500); go(); await sleep(800);
  const t = [...target.querySelectorAll('.markdown *')].find(e => e.children.length === 0 && /preview comment on md/.test(e.textContent));
  if (t) t.scrollIntoView({ block: 'center' });
  window.__res = t ? 'found' : 'none';
})(); 'started'
