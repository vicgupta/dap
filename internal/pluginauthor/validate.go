package pluginauthor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/declarative"
	"github.com/vicgupta/dap/internal/mail"
	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
	"github.com/vicgupta/dap/internal/pluginhost"
)

// Check is one validator result an agent can fix.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Report is the validate command's JSON.
type Report struct {
	OK     bool    `json:"ok"`
	Path   string  `json:"path"`
	Name   string  `json:"name,omitempty"`
	Checks []Check `json:"checks"`
}

// Validate runs the schema, the manifest rules, and the protocol harness.
func Validate(dir string) Report {
	return ValidateIn(dir, "")
}

// ValidateIn is Validate with temp files and npm and pip caches inside dataDir.
func ValidateIn(dir, dataDir string) Report {
	rep := Report{Path: dir, OK: true}
	add := func(c Check) {
		if !c.OK {
			rep.OK = false
		}
		rep.Checks = append(rep.Checks, c)
	}
	raw, err := os.ReadFile(dir + "/plugin.yaml")
	if err != nil {
		add(Check{Name: "manifest", OK: false, Detail: err.Error()})
		return rep
	}
	if declarative.LooksLike(raw) {
		return validateDeclarative(dir, raw)
	}
	schemaErrs := SchemaErrors(raw)
	if len(schemaErrs) > 0 {
		add(Check{Name: "schema", OK: false, Detail: strings.Join(schemaErrs, "; ")})
	} else {
		add(Check{Name: "schema", OK: true, Detail: "plugin.yaml matches schemas/plugin.schema.json"})
	}
	m, err := manifest.Parse(raw, "")
	if err != nil {
		add(Check{Name: "manifest", OK: false, Detail: err.Error()})
		return rep
	}
	rep.Name = m.Name
	add(Check{Name: "manifest", OK: true, Detail: m.Type + " " + m.Version})
	if !rep.OK {
		return rep
	}
	runDir := dir
	if missingSDK(dir) {
		staged, err := stageForValidate(dir, dataDir)
		if err != nil {
			add(Check{Name: "sdk", OK: false, Detail: err.Error()})
			return rep
		}
		runDir = staged
		add(Check{Name: "sdk", OK: true, Detail: "staged vendor/plugin-sdk for validation"})
	}
	if detail, err := prepareBuild(runDir, dataDir); err != nil {
		add(Check{Name: "build", OK: false, Detail: err.Error()})
		return rep
	} else if detail != "" {
		add(Check{Name: "build", OK: true, Detail: detail})
	}
	m, err = manifest.Parse(raw, runDir)
	if err != nil {
		add(Check{Name: "manifest", OK: false, Detail: err.Error()})
		return rep
	}
	argv, err := m.Launch(runDir)
	if err != nil {
		add(Check{Name: "start", OK: false, Detail: err.Error()})
		return rep
	}
	stub, err := startStubAPI(m.Name)
	if err != nil {
		add(Check{Name: "start", OK: false, Detail: err.Error()})
		return rep
	}
	defer stub.Close()
	host := pluginhost.New()
	defer host.StopAll()
	if err := host.Start(pluginhost.Options{
		Name:    m.Name,
		Dir:     runDir,
		Argv:    argv,
		Env:     pluginhost.PluginEnvIn(dataDir, "DAP_PLUGIN="+m.Name),
		DataDir: runDir,
	}); err != nil {
		add(Check{Name: "start", OK: false, Detail: err.Error()})
		return rep
	}
	add(Check{Name: "start", OK: true, Detail: strings.Join(argv, " ")})

	cfg, err := manifest.ResolveConfig(m, nil)
	if err != nil {
		add(Check{Name: "initialize", OK: false, Detail: err.Error()})
		return rep
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var init pluginhost.InitResult
	err = host.Call(ctx, m.Name, pluginhost.MethodInitialize, pluginhost.InitParams{
		Interface:  "dap.plugin.v1",
		DAPVersion: "0.1.0",
		Config:     cfg,
		PluginDir:  runDir,
		DataDir:    runDir,
		APIBase:    stub.base,
		Token:      conformanceToken,
	}, &init)
	if err != nil {
		add(Check{Name: "initialize", OK: false, Detail: err.Error() + stderrHint(host, m.Name)})
		return rep
	}
	if !init.Ready || (init.Interface != "" && init.Interface != "dap.plugin.v1") || !healthOK(init.Health.Status) {
		add(Check{Name: "initialize", OK: false, Detail: fmt.Sprintf("ready=%v interface=%q health=%q", init.Ready, init.Interface, init.Health.Status)})
		return rep
	}
	add(Check{Name: "initialize", OK: true, Detail: init.Health.Message})

	var watch pluginhost.WatchResult
	if err := host.Call(ctx, m.Name, pluginhost.MethodWatch, pluginhost.WatchParams{}, &watch); err != nil {
		add(Check{Name: "watch", OK: false, Detail: err.Error() + stderrHint(host, m.Name)})
		return rep
	}
	if watch.Observations == nil {
		watch.Observations = []model.Observation{}
	}
	add(Check{Name: "watch", OK: true, Detail: fmt.Sprintf("%d observations", len(watch.Observations))})

	var proposed pluginhost.ProposeResult
	if err := host.Call(ctx, m.Name, pluginhost.MethodPropose, pluginhost.ProposeParams{Observations: watch.Observations}, &proposed); err != nil {
		add(Check{Name: "propose", OK: false, Detail: err.Error() + stderrHint(host, m.Name)})
		return rep
	}
	if msg := checkCards(proposed.Cards); msg != "" {
		add(Check{Name: "propose", OK: false, Detail: msg})
		return rep
	}
	add(Check{Name: "propose", OK: true, Detail: fmt.Sprintf("%d cards", len(proposed.Cards))})

	var acted model.ActResult
	card := model.Card{ID: "c_conformance", Plugin: m.Name, Title: "Conformance", Kind: model.KindAction, Status: model.StatusActing}
	if err := host.Call(ctx, m.Name, pluginhost.MethodAct, pluginhost.ActParams{Card: card}, &acted); err != nil {
		add(Check{Name: "act", OK: false, Detail: err.Error() + stderrHint(host, m.Name)})
		return rep
	}
	if acted.Status != "done" && acted.Status != "failed" {
		add(Check{Name: "act", OK: false, Detail: "status must be done or failed"})
		return rep
	}
	if strings.TrimSpace(acted.Summary) == "" {
		add(Check{Name: "act", OK: false, Detail: "summary is required"})
		return rep
	}
	add(Check{Name: "act", OK: true, Detail: acted.Status})

	var report pluginhost.ReportResult
	if err := host.Call(ctx, m.Name, pluginhost.MethodReport, pluginhost.ReportParams{Period: "week", From: time.Now().Add(-time.Hour), To: time.Now()}, &report); err != nil {
		add(Check{Name: "report", OK: false, Detail: err.Error()})
		return rep
	}
	add(Check{Name: "report", OK: true, Detail: fmt.Sprintf("%d metrics", len(report.Metrics))})

	err = host.Call(ctx, m.Name, "nope", map[string]any{}, nil)
	if err == nil {
		add(Check{Name: "unknown_method", OK: false, Detail: "unknown method nope must return a JSON-RPC error"})
		return rep
	}
	add(Check{Name: "unknown_method", OK: true, Detail: "nope returned an error"})
	return rep
}

func healthOK(status string) bool {
	return status == model.HealthOK || status == model.HealthDegraded || status == model.HealthError
}

func checkCards(cards []model.CardDraft) string {
	for i, c := range cards {
		if strings.TrimSpace(c.Title) == "" {
			return fmt.Sprintf("card %d is missing a title", i)
		}
		if c.Kind != "" && c.Kind != model.KindAction && c.Kind != model.KindChange {
			return fmt.Sprintf("card %d kind must be action or change", i)
		}
		for _, e := range c.Effects {
			if e != model.EffectSends && e != model.EffectSpends && e != model.EffectDeletes {
				return fmt.Sprintf("card %d effect %q is not sends, spends, or deletes", i, e)
			}
		}
	}
	return ""
}

func validateDeclarative(dir string, raw []byte) Report {
	rep := Report{Path: dir, OK: true}
	add := func(c Check) {
		if !c.OK {
			rep.OK = false
		}
		rep.Checks = append(rep.Checks, c)
	}
	errs := declarative.SchemaErrors(raw)
	if len(errs) > 0 {
		add(Check{Name: "schema", OK: false, Detail: strings.Join(errs, "; ")})
		return rep
	}
	add(Check{Name: "schema", OK: true, Detail: "plugin.yaml matches schemas/declarative.schema.json"})
	m, err := manifest.Parse(raw, dir)
	if err != nil {
		add(Check{Name: "manifest", OK: false, Detail: err.Error()})
		return rep
	}
	rep.Name = m.Name
	var names []string
	for _, p := range m.Permissions {
		names = append(names, p.Name+" ("+p.Mode+")")
	}
	add(Check{Name: "manifest", OK: true, Detail: "declarative " + m.Version})
	add(Check{Name: "permissions", OK: true, Detail: strings.Join(names, ", ")})
	if m.Declarative == nil {
		add(Check{Name: "dry_run", OK: false, Detail: "declarative spec is missing"})
		return rep
	}
	_, text, err := declarative.Sample(*m.Declarative, nil, time.Now(), mail.Seed(time.Now()))
	if err != nil {
		add(Check{Name: "dry_run", OK: false, Detail: err.Error()})
		return rep
	}
	line := text
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	add(Check{Name: "dry_run", OK: true, Detail: line})
	return rep
}

func stderrHint(h *pluginhost.Host, name string) string {
	tail := strings.TrimSpace(h.Stderr(name))
	if tail == "" {
		return ""
	}
	if len(tail) > 500 {
		tail = tail[len(tail)-500:]
	}
	return "\nstderr: " + tail
}

// Must be referenced so a future encoder change does not drop fields.
var _ = json.Marshal
