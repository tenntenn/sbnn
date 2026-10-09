window.__lt = []; window.__t0 = performance.now();
try { new PerformanceObserver(l => { for (const e of l.getEntries()) window.__lt.push([Math.round(e.startTime), Math.round(e.duration)]) }).observe({type:'longtask', buffered:true}) } catch(e){}
window.__fcp = null;
try { new PerformanceObserver(l => { for (const e of l.getEntries()) if (e.name==='first-contentful-paint') window.__fcp = Math.round(e.startTime) }).observe({type:'paint', buffered:true}) } catch(e){}
(function poll(){ const t=document.querySelector('.diff-stack table'); if(t){ window.__firstTable=Math.round(performance.now()); } else requestAnimationFrame(poll) })();
