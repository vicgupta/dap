/* Apply dap.theme before the first stylesheet paints.
   Order: ?theme= query, then localStorage, then the data-theme already on <html>. */
(function () {
  var allowed = { system: 1, light: 1, dark: 1 };
  function fromQuery() {
    try {
      var q = new URLSearchParams(location.search).get("theme");
      if (q && allowed[q]) return q;
    } catch (e) {}
    return "";
  }
  function fromStore() {
    try {
      var s = localStorage.getItem("dap.theme");
      if (s && allowed[s]) return s;
    } catch (e) {}
    return "";
  }
  var query = fromQuery();
  var theme = query || fromStore() || document.documentElement.getAttribute("data-theme") || "system";
  if (!allowed[theme]) theme = "system";
  document.documentElement.setAttribute("data-theme", theme);
  if (query) {
    try { localStorage.setItem("dap.theme", theme); } catch (e) {}
  }
  window.addEventListener("message", function (ev) {
    var data = ev.data;
    if (!data || data.type !== "dap.theme" || !allowed[data.theme]) return;
    var origin = ev.origin || "";
    if (!/^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(origin)) return;
    document.documentElement.setAttribute("data-theme", data.theme);
    try { localStorage.setItem("dap.theme", data.theme); } catch (e) {}
  });
})();
