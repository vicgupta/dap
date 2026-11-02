package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vicgupta/dap/internal/guardrails"
	"github.com/vicgupta/dap/internal/ledger"
)

// ConnectExecute runs one tool from the Composio registry.
// The plugin must have declared the tool's permission. A send, spend, or delete
// also needs an approved card. Arguments are not written to the ledger.
func (a *App) ConnectExecute(ctx context.Context, plugin, toolkit, tool string, args map[string]any, cardID string) (json.RawMessage, error) {
	reg, err := a.mail.Registry()
	if err != nil {
		return nil, err
	}
	tk, spec, ok := reg.Tool(toolkit, tool)
	if !ok {
		return nil, fmt.Errorf("%s is not a declared tool", tool)
	}
	if err := a.gate(plugin, spec.Permission, spec.Effects, cardID, true); err != nil {
		return nil, err
	}
	raw, err := a.mail.Execute(ctx, tk.ID, tool, args)
	if err != nil {
		return nil, err
	}
	if guardrails.Sensitive(spec.Effects) {
		a.log(ledger.Entry{Plugin: plugin, Kind: "connect.exec", Message: "Executed " + tool})
	}
	return raw, nil
}
