window.__res = null;
(async () => {
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const frame = () => new Promise(r => requestAnimationFrame(() => r()));
  const st = document.querySelector('.diff-stack');
  let sc = st; while (sc && !(sc.scrollHeight > sc.clientHeight + 5 && /auto|scroll/.test(getComputedStyle(sc).overflowY))) sc = sc.parentElement;
  const secs = () => [...st.querySelectorAll('.file-section')];
  const blankVisible = () => {
    const pr = sc.getBoundingClientRect(); let n = 0;
    for (const s of secs()) { const r = s.getBoundingClientRect(); if (r.bottom > pr.top && r.top < pr.bottom && s.children.length === 0) n++; }
    return n;
  };
  const mountedCount = () => secs().filter(s => s.children.length > 0).length;
  const activeOk = () => {
    // the active sidebar row must belong to a section that reaches into the top 70% band
    const pr = sc.getBoundingClientRect();
    const item = document.querySelector('.sidebar .file-item.active');
    if (!item) return 'no-active';
    const path = (item.textContent || '');
    const ok = secs().some(s => { const r = s.getBoundingClientRect(); return r.top < pr.top + pr.height * 0.7 && r.bottom > pr.top && s.textContent && s.textContent.includes(path.replace(/^check|^[a-z_]+(?=docs|pkg)/, '').slice(0, 14)); });
    return ok ? 'ok' : 'MISMATCH:' + path.slice(0, 40);
  };
  const out = { scrollHeight: sc.scrollHeight };
  // 1. fast scroll top to bottom
  sc.scrollTop = 0; await sleep(500);
  let frames = 0, blankFrames = 0, streak = 0, maxStreak = 0, maxMounted = 0, maxBlank = 0;
  const t0 = performance.now(); const longBefore = window.__lt ? window.__lt.length : 0;
  while (sc.scrollTop + sc.clientHeight < sc.scrollHeight - 5 && performance.now() - t0 < 60000) {
    sc.scrollTop += 2500; await frame(); frames++;
    const b = blankVisible(); maxBlank = Math.max(maxBlank, b);
    if (b > 0) { blankFrames++; streak++; maxStreak = Math.max(maxStreak, streak); } else streak = 0;
    maxMounted = Math.max(maxMounted, mountedCount());
  }
  out.fastScroll = { frames, blankFrames, maxBlankStreakFrames: maxStreak, maxBlankSections: maxBlank, maxMounted, ms: Math.round(performance.now() - t0) };
  await sleep(1000);
  out.afterFastScroll = { blankVisible: blankVisible(), mounted: mountedCount(), nodes: document.getElementsByTagName('*').length, active: activeOk() };
  // 2. jumps through the sidebar
  const items = () => [...document.querySelectorAll('.sidebar .file-item')];
  out.itemCount = items().length;
  out.jumps = [];
  for (const i of [150, 399, 10, 250, 0, 399, 123]) {
    const it = items()[i]; if (!it) { out.jumps.push({ i, err: 'no item' }); continue; }
    const label = it.textContent.slice(0, 40);
    it.click();
    const tj = performance.now(); let blankFor = 0;
    for (let k = 0; k < 30; k++) { await sleep(50); if (blankVisible() > 0) blankFor = Math.round(performance.now() - tj); }
    await sleep(500);
    const act = document.querySelector('.sidebar .file-item.active');
    const same = act && act === items()[i];
    out.jumps.push({ i, label, activeIsClicked: !!same, blankAfter: blankVisible(), blankLastSeenMs: blankFor, topOffset: Math.round(secs()[i].getBoundingClientRect().top - sc.getBoundingClientRect().top), mounted: mountedCount(), scrollTop: Math.round(sc.scrollTop) });
  }
  // 3. random positions
  out.random = [];
  for (let k = 0; k < 8; k++) {
    sc.scrollTop = Math.floor(Math.random() * (sc.scrollHeight - sc.clientHeight));
    await sleep(700);
    out.random.push({ blank: blankVisible(), active: activeOk() });
  }
  window.__res = JSON.stringify(out, null, 1);
})(); 'started'
