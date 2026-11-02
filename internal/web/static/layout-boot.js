/* Apply the saved card layout before the first stylesheet paints.
   Order: ?density= query, then localStorage["dap.layout"], then
   <meta name="dap-layout">, then the data attributes already on <html>.
   See docs/layout.md. */
(function () {
  var DENSITY = { comfortable: 1, compact: 1 };
  var HOME = { grid: 1, list: 1, board: 1 };
  var INBOX = { list: 1, grid: 1, board: 1, split: 1 };

  function parse(raw) {
    try { return JSON.parse(raw); } catch (e) { return null; }
  }
  function route() {
    var h = (location.hash || "").replace(/^#\/?/, "");
    return h === "inbox" ? "inbox" : "home";
  }
  function fromQuery() {
    try {
      var q = new URLSearchParams(location.search).get("density");
      if (q && DENSITY[q]) return q;
    } catch (e) {}
    return "";
  }
  function fromStore() {
    try { return parse(localStorage.getItem("dap.layout")); } catch (e) { return null; }
  }
  function fromMeta() {
    var el = document.querySelector('meta[name="dap-layout"]');
    return el && el.content ? parse(el.content) : null;
  }

  var query = fromQuery();
  var prefs = fromStore() || fromMeta();
  var root = document.documentElement;
  if (!prefs) {
    prefs = {
      home: {
        view: HOME[root.getAttribute("data-home-view")] ? root.getAttribute("data-home-view") : "grid",
        density: "comfortable",
      },
      inbox: {
        view: INBOX[root.getAttribute("data-inbox-view")] ? root.getAttribute("data-inbox-view") : "list",
        density: "comfortable",
      },
    };
  }
  var key = route();
  var density = query || (prefs[key] && prefs[key].density) || (prefs.home && prefs.home.density) || root.getAttribute("data-density") || "comfortable";
  if (!DENSITY[density]) density = "comfortable";
  var homeView = prefs.home && HOME[prefs.home.view] ? prefs.home.view : "grid";
  var inboxView = prefs.inbox && INBOX[prefs.inbox.view] ? prefs.inbox.view : "list";
  root.setAttribute("data-density", density);
  root.setAttribute("data-home-view", homeView);
  root.setAttribute("data-inbox-view", inboxView);
  if (query) {
    try { localStorage.setItem("dap.density", density); } catch (e) {}
  }
  window.__DAP_LAYOUT__ = prefs;

  window.addEventListener("message", function (ev) {
    var data = ev.data;
    if (!data || data.type !== "dap.layout" || !DENSITY[data.density]) return;
    var origin = ev.origin || "";
    if (!/^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(origin)) return;
    root.setAttribute("data-density", data.density);
    try { localStorage.setItem("dap.density", data.density); } catch (e) {}
  });

  window.addEventListener("hashchange", function () {
    if (query) return;
    var live = window.__DAP_LAYOUT__ || prefs;
    var k = route();
    var d = live[k] && live[k].density;
    if (DENSITY[d]) root.setAttribute("data-density", d);
  });
})();
