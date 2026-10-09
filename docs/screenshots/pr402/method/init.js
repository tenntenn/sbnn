window.__lt = []; window.__ltIdx = 0;
try { new PerformanceObserver(l => { for (const e of l.getEntries()) window.__lt.push([Math.round(e.startTime), Math.round(e.duration)]) }).observe({type:'longtask', buffered:true}) } catch(e){}
window.__es = {created:0, open:0, errors:0, opens:0, closed:0, list:[]};
(function(){
  const Orig = window.EventSource;
  if (!Orig) return;
  window.EventSource = function(url, opts) {
    const es = new Orig(url, opts);
    window.__es.created++; window.__es.list.push(es);
    es.addEventListener('open', () => { window.__es.opens++; });
    es.addEventListener('error', () => { window.__es.errors++; });
    return es;
  };
  window.EventSource.prototype = Orig.prototype;
  window.EventSource.CONNECTING = 0; window.EventSource.OPEN = 1; window.EventSource.CLOSED = 2;
})();
