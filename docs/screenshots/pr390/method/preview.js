window.__res = null;
(async () => {
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const pv = document.querySelector('.preview-stack');
  let sc = pv; while (sc && !(sc.scrollHeight > sc.clientHeight + 5 && /auto|scroll/.test(getComputedStyle(sc).overflowY))) sc = sc.parentElement;
  const out = { scroller: sc && sc.className };
  const secs = () => [...pv.querySelectorAll('.file-section')];
  const key = secs().find(s => s.id.includes('d10') && true);
  const mdSecs = () => secs().filter(s => s.querySelector('.preview-header .path') && /r10\/doc1\.md/.test(s.querySelector('.preview-header .path').textContent));
  const target = mdSecs()[0];
  const go = () => { sc.scrollTop += target.getBoundingClientRect().top - sc.getBoundingClientRect().top - 50; };
  out.targetFound = !!target;
  const state = () => ({ body: !!target.querySelector('.preview-body'), unloaded: !!target.querySelector('.preview-body-unloaded'), md: !!target.querySelector('.markdown'), threads: target.querySelectorAll('.markdown .comment, .markdown [id^="pc-"], .markdown .comment-thread').length, h: Math.round(target.getBoundingClientRect().height) });
  go(); await sleep(1500); go(); await sleep(1000);
  out.atTarget = state();
  out.slotsAtTarget = target.querySelectorAll('.markdown .comment-slot, .markdown [data-comment-slot]').length;
  const heightBefore = target.getBoundingClientRect().height;
  sc.scrollTop = 0; await sleep(1500);
  out.farAway = state();
  go(); await sleep(1500);
  out.back = state();
  out.heightRestoredWithin2px = Math.abs(target.getBoundingClientRect().height - heightBefore) <= 2;
  out.html = (target.querySelector('.markdown') || document.body).innerHTML.length;
  out.commentTexts = [...target.querySelectorAll('.markdown *')].filter(e => e.children.length === 0 && /preview comment|second preview/.test(e.textContent)).map(e => e.textContent.slice(0, 40));
  out.previewNodes = pv.getElementsByTagName('*').length;
  window.__res = JSON.stringify(out, null, 1);
})(); 'started'
