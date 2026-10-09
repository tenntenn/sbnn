(async () => {
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true, bubbles: true }));
  await sleep(9000);
  window.getSelection().removeAllRanges();
  const found = window.find('zzmdneedle400', true, false, true);
  const sel = window.getSelection();
  if (sel.rangeCount) sel.getRangeAt(0).startContainer.parentElement.scrollIntoView({ block: 'center' });
  await sleep(500);
  return JSON.stringify({ found, text: String(sel) });
})()
