(async () => {
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const st = document.querySelector('.diff-stack');
  let sc = st; while (sc && !(sc.scrollHeight > sc.clientHeight + 5 && /auto|scroll/.test(getComputedStyle(sc).overflowY))) sc = sc.parentElement;
  const secs = () => [...st.querySelectorAll('.file-section')];
  const mountedN = () => secs().filter(s => s.children.length > 0).length;
  const out = {};
  sc.scrollTop = 0; await sleep(800);
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, bubbles: true }));
  const t0 = performance.now();
  while (mountedN() < secs().length && performance.now() - t0 < 30000) await sleep(100);
  out.mountedDuringHold = mountedN();
  // open a comment form in a section that is far from the viewport
  const sec = secs()[300];
  const cell = [...sec.querySelectorAll('td.num.clickable')].find(c => c.textContent.trim() !== '');
  const fire = (t, type) => t.dispatchEvent(new PointerEvent(type, { bubbles: true, cancelable: true, button: 0, pointerId: 1, isPrimary: true }));
  fire(cell, 'pointerdown'); fire(cell, 'pointerup'); cell.dispatchEvent(new MouseEvent('click', { bubbles: true, button: 0 }));
  await sleep(500);
  const ta = sec.querySelector('textarea');
  out.formOpened = !!ta;
  if (ta) {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(ta, 'half typed draft');
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  }
  document.activeElement && document.activeElement.blur && document.activeElement.blur();
  await sleep(300);
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await sleep(2000);
  out.afterEscape = { mounted: mountedN(), draftSectionMounted: sec.children.length > 0, draftValue: sec.querySelector('textarea') && sec.querySelector('textarea').value };
  return JSON.stringify(out, null, 1);
})()
