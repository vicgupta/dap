const assert = require("assert");
const L = require("./layout-logic.js");

const def = L.normalize(null);
assert.strictEqual(def.home.view, "grid");
assert.strictEqual(def.home.density, "comfortable");
assert.strictEqual(def.inbox.view, "list");

const coerced = L.normalize({ home: { view: "split", density: "roomy" }, inbox: { view: "BOARD" } });
assert.strictEqual(coerced.home.view, "grid");
assert.strictEqual(coerced.inbox.view, "board");

assert.strictEqual(L.resolveSize("small", "large", "medium"), "medium");
assert.strictEqual(L.resolveSize("", "large", "medium"), "large");
assert.strictEqual(L.resolveSize("large", "small", ""), "large");

const items = [{ id: "a" }, { id: "b" }, { id: "c" }];
const pinned = L.arrange(items, (x) => x.id, { order: ["a", "b", "c"], pinned: ["b"] }).map((x) => x.id);
assert.deepStrictEqual(pinned, ["b", "a", "c"]);

const moved = L.reorder({ order: ["a", "b", "c"], pinned: [] }, ["a", "b", "c"], "c", "a");
assert.deepStrictEqual(moved.order, ["c", "a", "b"]);
assert.deepStrictEqual(moved.pinned, []);

const across = L.reorder({ order: ["b", "a", "c"], pinned: ["b"] }, ["b", "a", "c"], "a", "b");
assert.deepStrictEqual(across.order, ["a", "b", "c"]);
assert.deepStrictEqual(across.pinned, ["a", "b"]);

const kept = L.reorder({ pinned: ["gone", "b"], order: [] }, ["b", "a"], "a", "b");
assert.ok(kept.pinned.indexOf("gone") >= 0);

console.log("layout-logic ok");
