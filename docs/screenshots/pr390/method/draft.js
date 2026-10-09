window.__res = null;
(async () => {
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const st = document.querySelector('.diff-stack');
  let sc = st; while (sc && !(sc.scrollHeight > sc.clientHeight + 5 && /auto|scroll/.test(getComputedStyle(sc).overflowY))) sc = sc.parentElement;
  const out = {};
  // go to file 150 through the sidebar
  document.querySelectorAll('.sidebar .file-item')[150].click(); await sleep(1200);
  const sec = [...st.querySelectorAll('.file-section')][150];
  const cell = [...sec.querySelectorAll('td.num.clickable')].find(c => c.textContent.trim() !== '');
  out.cellFound = !!cell;
  const fire = (t, type) => t.dispatchEvent(new PointerEvent(type, { bubbles: true, cancelable: true, button: 0, pointerId: 1, isPrimary: true }));
  fire(cell, 'pointerdown'); fire(cell, 'pointerup'); cell.dispatchEvent(new MouseEvent('click', { bubbles: true, button: 0 }));
  await sleep(500);
  let ta = sec.querySelector('textarea');
  out.formOpened = !!ta;
  if (ta) {
    const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, 'half typed draft'); ta.dispatchEvent(new Event('input', { bubbles: true }));
    await sleep(200);
  }
  // scroll far away and wait
  sc.scrollTop = 0; await sleep(1500);
  out.farAway = { sectionStillMounted: sec.children.length > 0, textarea: !!sec.querySelector('textarea'), value: sec.querySelector('textarea') && sec.querySelector('textarea').value, mountedTotal: [...st.querySelectorAll('.file-section')].filter(s => s.children.length).length };
  // blur it, the section may now be released
  document.activeElement && document.activeElement.blur && document.activeElement.blur(); await sleep(800);
  out.afterBlur = { textarea: !!sec.querySelector('textarea'), mounted: sec.children.length > 0 };
  // cancel via Escape not needed; scroll back
  sc.scrollTop = sec.offsetTop; await sleep(800);
  window.__res = JSON.stringify(out, null, 1);
})(); 'started'
