# scheduled-checklist

Starting point built on the workflow plugin. Customize the steps. Do not bypass guardrails.

- Delete the DAP_SCAFFOLD_STARTER line once this starter is no longer the idle copy.
- Update plugin.yaml so the description and the steps match what you were asked to build.
- Stdout is JSON-RPC only. Logs go to stderr.
- A step whose effects include `sends`, `spends`, or `deletes` must propose a card with that effect so it pauses for approval.
