const assert = require("assert");
const C = require("./create-logic.js");

assert.strictEqual(C.outcome({ status: "running" }).step, "run");
assert.strictEqual(C.outcome({ status: "running" }).propose, false);

const failed = C.outcome({ status: "failed", message: "The coding agent stopped with an error, so DAP did not propose an install." });
assert.strictEqual(failed.step, "error");
assert.strictEqual(failed.propose, false);
assert.strictEqual(failed.retry, true);
assert.ok(failed.message.includes("did not propose"));

const cancelled = C.outcome({ status: "cancelled" });
assert.strictEqual(cancelled.retry, true);
assert.ok(cancelled.message.includes("Nothing was proposed"));

const unchanged = C.outcome({ status: "finished", changed: false, message: "left the starter" });
assert.strictEqual(unchanged.step, "error");
assert.strictEqual(unchanged.propose, false);
assert.strictEqual(unchanged.message, "left the starter");

const ready = C.outcome({ status: "finished", changed: true });
assert.strictEqual(ready.step, "check");
assert.strictEqual(ready.propose, false);

assert.strictEqual(C.afterValidation({ ok: true }).step, "propose");
assert.strictEqual(C.afterValidation({ ok: true }).propose, true);
assert.strictEqual(C.afterValidation({ ok: false }).propose, false);
assert.strictEqual(C.afterValidation({ ok: false }).step, "fix");

assert.strictEqual(C.logTail("a\nb\nc", 2), "b\nc");
assert.strictEqual(C.logTail("a\nb\n", 5), "a\nb");
assert.strictEqual(C.logTail("", 3), "");
