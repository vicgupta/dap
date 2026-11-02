/* Create-plugin decisions. Shared by app.js and node tests.
   The page proposes an install only after the agent finishes and the draft changed. */
(function (root, factory) {
  var api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  root.DAPCreate = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  function logTail(log, n) {
    var lines = String(log || "").split("\n");
    if (lines.length && lines[lines.length - 1] === "") lines.pop();
    n = n || 40;
    if (lines.length <= n) return lines.join("\n");
    return lines.slice(lines.length - n).join("\n");
  }

  function outcome(run) {
    run = run || {};
    var status = String(run.status || "");
    if (status === "" || status === "running" || status === "checking") {
      return { step: status === "checking" ? "check" : "run", propose: false, retry: false };
    }
    if (status === "cancelled") {
      return {
        step: "error",
        propose: false,
        retry: true,
        title: "Cancelled",
        message: run.message || "Cancelled. Nothing was proposed.",
      };
    }
    if (status !== "finished") {
      return {
        step: "error",
        propose: false,
        retry: true,
        title: "The agent stopped",
        message: run.message || "The coding agent did not finish the plugin, so DAP did not propose an install.",
      };
    }
    if (run.changed === false) {
      return {
        step: "error",
        propose: false,
        retry: true,
        title: "Nothing was proposed",
        message: run.message || "The agent finished without changing the starter plugin, so DAP did not propose an install.",
      };
    }
    return { step: "check", propose: false, retry: false };
  }

  function afterValidation(report) {
    if (report && report.ok) return { step: "propose", propose: true, retry: false };
    return {
      step: "fix",
      propose: false,
      retry: false,
      title: "Needs a fix",
      message: "Validation did not pass, so DAP did not propose an install.",
    };
  }

  return { logTail: logTail, outcome: outcome, afterValidation: afterValidation };
});
