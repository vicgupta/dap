/* Card order, density, and tile size. Shared by app.js and node tests.
   The rules match internal/layout. */
(function (root, factory) {
  var api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  root.DAPLayout = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  var HOME = { grid: 1, list: 1, board: 1 };
  var INBOX = { list: 1, grid: 1, board: 1, split: 1 };
  var DENSITY = { comfortable: 1, compact: 1 };
  var SIZE = { small: 1, medium: 2, large: 3 };

  function cleanIds(ids) {
    var out = [];
    var seen = {};
    var list = ids || [];
    for (var i = 0; i < list.length; i++) {
      var id = String(list[i] == null ? "" : list[i]).trim();
      if (!id || id.length > 128 || /[\r\n\t]/.test(id) || seen[id]) continue;
      seen[id] = 1;
      out.push(id);
      if (out.length === 200) break;
    }
    return out;
  }

  function one(raw, allowed, fallback) {
    raw = raw || {};
    var view = String(raw.view || "").trim().toLowerCase();
    if (!allowed[view]) view = fallback;
    var density = String(raw.density || "").trim().toLowerCase();
    if (!DENSITY[density]) density = "comfortable";
    var sizes = {};
    var src = raw.sizes || {};
    var keys = Object.keys(src).sort();
    var n = 0;
    for (var i = 0; i < keys.length && n < 200; i++) {
      var id = String(keys[i] || "").trim();
      var size = String(src[keys[i]] || "").trim().toLowerCase();
      if (!id || id.length > 128 || /[\r\n\t]/.test(id) || !SIZE[size]) continue;
      sizes[id] = size;
      n++;
    }
    var out = {
      view: view,
      density: density,
      order: cleanIds(raw.order),
      pinned: cleanIds(raw.pinned),
      collapsed: cleanIds(raw.collapsed),
    };
    if (n) out.sizes = sizes;
    return out;
  }

  function normalize(raw) {
    raw = raw || {};
    return { home: one(raw.home, HOME, "grid"), inbox: one(raw.inbox, INBOX, "list") };
  }

  function resolveSize(user, preferred, min) {
    var floor = SIZE[min] ? min : "small";
    var size = user && SIZE[user] ? user : (preferred && SIZE[preferred] ? preferred : "medium");
    if (SIZE[size] < SIZE[floor]) return floor;
    return size;
  }

  function arrange(items, idOf, prefs) {
    prefs = prefs || {};
    var pinned = {};
    (prefs.pinned || []).forEach(function (id) { pinned[id] = 1; });
    var rank = {};
    (prefs.order || []).forEach(function (id, i) { if (rank[id] == null) rank[id] = i; });
    var copy = items.slice();
    copy.sort(function (a, b) {
      var ia = idOf(a);
      var ib = idOf(b);
      var ap = pinned[ia] ? 0 : 1;
      var bp = pinned[ib] ? 0 : 1;
      if (ap !== bp) return ap - bp;
      var ar = rank[ia] == null ? 1e15 : rank[ia];
      var br = rank[ib] == null ? 1e15 : rank[ib];
      if (ar !== br) return ar - br;
      return 0;
    });
    return copy;
  }

  function move(ids, fromId, toId) {
    var list = ids.slice();
    var from = list.indexOf(fromId);
    var to = list.indexOf(toId);
    if (from < 0 || to < 0 || from === to) return list;
    var item = list.splice(from, 1)[0];
    list.splice(to, 0, item);
    return list;
  }

  /* Dragging across the pin boundary pins or unpins the card so the drop
     position sticks. Pins always render as a prefix. */
  function reorder(prefs, visualIds, fromId, toId) {
    prefs = prefs || {};
    var next = move(visualIds, fromId, toId);
    var pinned = {};
    (prefs.pinned || []).forEach(function (id) { pinned[id] = 1; });
    var fromPinned = !!pinned[fromId];
    var toPinned = !!pinned[toId];
    if (fromPinned !== toPinned) {
      if (fromPinned) delete pinned[fromId];
      else pinned[fromId] = 1;
    }
    var kept = (prefs.pinned || []).filter(function (id) {
      return visualIds.indexOf(id) < 0 && id !== fromId;
    });
    return { order: next, pinned: kept.concat(next.filter(function (id) { return pinned[id]; })) };
  }

  return { normalize: normalize, arrange: arrange, reorder: reorder, resolveSize: resolveSize };
});
