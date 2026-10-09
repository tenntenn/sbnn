(async () => {
  await new Promise(r => setTimeout(r, 6000));
  const lt = window.__lt, total = lt.reduce((a,x)=>a+x[1],0), max = lt.reduce((a,x)=>Math.max(a,x[1]),0);
  return JSON.stringify({
    nodes: document.getElementsByTagName('*').length,
    longtaskTotalMs: total, longtaskMaxMs: max, longtasks: lt.length,
    heapMB: Math.round(performance.memory.usedJSHeapSize/1048576),
    firstTable: window.__firstTable, fcp: window.__fcp,
    sections: document.querySelectorAll('.diff-stack .file-section').length,
    tables: document.querySelectorAll('.diff-stack table').length,
    previewMd: document.querySelectorAll('.preview-stack .markdown').length,
  })
})()
