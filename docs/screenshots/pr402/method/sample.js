(() => {
  if(!window.__hb){window.__hb={last:performance.now(),max:0};setInterval(()=>{const n=performance.now();const g=n-window.__hb.last-100;if(g>window.__hb.max)window.__hb.max=g;window.__hb.last=n},100)}
  const hbMax=Math.round(window.__hb.max);window.__hb.max=0;
  const lt = window.__lt || [];
  const from = window.__ltIdx || 0;
  const d = lt.slice(from);
  window.__ltIdx = lt.length;
  const es = window.__es || {};
  const act = document.querySelector('.file-item.active');
  const panes = document.querySelectorAll('.split-pane');
  return JSON.stringify({
    heapMB: +(performance.memory.usedJSHeapSize / 1048576).toFixed(1),
    heapTotalMB: +(performance.memory.totalJSHeapSize / 1048576).toFixed(1),
    nodes: document.getElementsByTagName('*').length,
    tables: document.querySelectorAll('.diff-stack table').length,
    sections: document.querySelectorAll('.diff-stack .file-section').length,
    sidebarItems: document.querySelectorAll('.file-item').length,
    previewMd: document.querySelectorAll('.preview-stack .markdown').length,
    loopLagMaxMs: hbMax,
    ltCount: d.length,
    ltTotalMs: d.reduce((a, x) => a + x[1], 0),
    ltMaxMs: d.reduce((a, x) => Math.max(a, x[1]), 0),
    esCreated: es.created, esStates: (es.list || []).map(e => e.readyState).join(''), esErrors: es.errors, esOpens: es.opens,
    active: act ? act.querySelector('.file-path').title : null,
    scrollTop: panes[0] ? Math.round(panes[0].scrollTop) : null,
    scrollHeight: panes[0] ? panes[0].scrollHeight : null,
    now: Date.now(),
  });
})()
