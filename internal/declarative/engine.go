package declarative

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/mailbox"
	"github.com/vicgupta/dap/internal/model"
)

const defaultLimit = 20

// Mailbox is the abstract email service. The core fills it from whichever
// mailbox is connected. Tests pass a fixed list.
type Mailbox interface {
	Providers(ctx context.Context) ([]string, error)
	Threads(ctx context.Context, provider, window string) ([]mailbox.Thread, error)
	Draft(ctx context.Context, req mailbox.SendRequest) (mailbox.DraftResult, error)
	Send(ctx context.Context, req mailbox.SendRequest) (mailbox.SendResult, error)
}

// State is scratch the core owns for one declarative plugin.
// Remove deletes it.
type State interface {
	Load() (Blob, error)
	Save(Blob) error
}

// Blob is the scratch document.
type Blob struct {
	Labels    []LabelMark `json:"labels,omitempty"`
	Reminders []Reminder  `json:"reminders,omitempty"`
	Runs      int         `json:"runs,omitempty"`
	Drafts    int         `json:"drafts,omitempty"`
	Sends     int         `json:"sends,omitempty"`
}

// LabelMark is a label recorded after approval. It is not a provider label.
type LabelMark struct {
	ThreadID string    `json:"thread_id"`
	Label    string    `json:"label"`
	At       time.Time `json:"at"`
}

// Reminder is recorded after approval. The inbox card is the reminder.
type Reminder struct {
	Title    string    `json:"title"`
	ThreadID string    `json:"thread_id,omitempty"`
	At       time.Time `json:"at"`
}

// Memory is an in-memory State for tests and the install dry run.
type Memory struct{ Blob Blob }

func (m *Memory) Load() (Blob, error) { return m.Blob, nil }
func (m *Memory) Save(b Blob) error   { m.Blob = b; return nil }

// Engine runs one declarative plugin inside the process.
type Engine struct {
	Plugin Plugin
	Config map[string]any
	Now    func() time.Time
	Mail   Mailbox
	State  State

	last Outcome
}

// Outcome is one watch/propose pass.
type Outcome struct {
	Tile         model.Tile
	Observations []model.Observation
	Cards        []model.CardDraft
	Metrics      []model.Metric
	Note         string
}

// New builds an engine. Now, Mail, and State may be nil until used.
func New(p Plugin, cfg map[string]any) *Engine {
	if cfg == nil {
		cfg = map[string]any{}
	}
	return &Engine{Plugin: p, Config: cfg, Now: time.Now, State: &Memory{}}
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Initialize reports the plugin ready. It does not read mail.
func (e *Engine) Initialize() (bool, string, string) {
	msg := "Ready"
	if e.Plugin.Description != "" {
		msg = e.Plugin.Description
	}
	return true, model.HealthOK, msg
}

// Watch reads services and remembers the cards Propose will return.
func (e *Engine) Watch(ctx context.Context, since *time.Time) (Outcome, error) {
	out, err := e.evaluate(ctx, since)
	if err != nil {
		return Outcome{}, err
	}
	e.last = out
	return out, nil
}

// Propose returns the cards from the last Watch.
func (e *Engine) Propose() Outcome {
	return e.last
}

// Report returns counters stored for this plugin.
func (e *Engine) Report() []model.Metric {
	b, _ := e.State.Load()
	return []model.Metric{
		{Key: "runs", Label: "Runs", Value: b.Runs},
		{Key: "drafts", Label: "Drafts", Value: b.Drafts},
		{Key: "sends", Label: "Sends", Value: b.Sends},
	}
}

// Act performs an approved card. Propose never calls this.
func (e *Engine) Act(ctx context.Context, card model.Card) (model.ActResult, error) {
	var body actPayload
	if len(card.Payload) > 0 {
		_ = json.Unmarshal(card.Payload, &body)
	}
	switch body.Op {
	case "draft":
		return e.actDraft(ctx, card, body)
	case "send":
		return e.actSend(ctx, card, body)
	case "label":
		return e.actLabel(body)
	case "reminder":
		return e.actReminder(card, body)
	case "report", "checklist", "":
		return model.ActResult{Status: "done", Summary: "Noted. Nothing was sent."}, nil
	default:
		return model.ActResult{Status: "failed", Summary: "Unknown action " + body.Op}, nil
	}
}

func (e *Engine) actDraft(ctx context.Context, card model.Card, body actPayload) (model.ActResult, error) {
	if e.Mail == nil {
		return model.ActResult{Status: "failed", Summary: "No mailbox is available."}, nil
	}
	text := strings.TrimSpace(card.Body)
	if text == "" {
		text = body.Body
	}
	_, err := e.Mail.Draft(ctx, mailbox.SendRequest{
		Provider:  body.Provider,
		ThreadID:  body.ThreadID,
		MessageID: body.MessageID,
		To:        body.To,
		Subject:   body.Subject,
		Body:      text,
	})
	if err != nil {
		return model.ActResult{Status: "failed", Summary: "Draft was not saved: " + err.Error()}, nil
	}
	b, _ := e.State.Load()
	b.Drafts++
	_ = e.State.Save(b)
	return model.ActResult{Status: "done", Summary: "Saved a draft to " + body.To + ". Nothing was sent."}, nil
}

func (e *Engine) actSend(ctx context.Context, card model.Card, body actPayload) (model.ActResult, error) {
	if e.Mail == nil {
		return model.ActResult{Status: "failed", Summary: "No mailbox is available."}, nil
	}
	text := strings.TrimSpace(card.Body)
	if text == "" {
		text = body.Body
	}
	res, err := e.Mail.Send(ctx, mailbox.SendRequest{
		Provider:  body.Provider,
		ThreadID:  body.ThreadID,
		MessageID: body.MessageID,
		To:        body.To,
		Subject:   body.Subject,
		Body:      text,
		CardID:    card.ID,
	})
	if err != nil {
		return model.ActResult{Status: "failed", Summary: "Not sent: " + err.Error()}, nil
	}
	b, _ := e.State.Load()
	b.Sends++
	_ = e.State.Save(b)
	return model.ActResult{Status: "done", Summary: "Sent to " + res.To + "."}, nil
}

func (e *Engine) actLabel(body actPayload) (model.ActResult, error) {
	b, err := e.State.Load()
	if err != nil {
		return model.ActResult{Status: "failed", Summary: err.Error()}, nil
	}
	b.Labels = append(b.Labels, LabelMark{ThreadID: body.ThreadID, Label: body.Label, At: e.now().UTC()})
	if err := e.State.Save(b); err != nil {
		return model.ActResult{Status: "failed", Summary: err.Error()}, nil
	}
	return model.ActResult{Status: "done", Summary: "Recorded label " + body.Label + "."}, nil
}

func (e *Engine) actReminder(card model.Card, body actPayload) (model.ActResult, error) {
	b, err := e.State.Load()
	if err != nil {
		return model.ActResult{Status: "failed", Summary: err.Error()}, nil
	}
	title := body.Title
	if title == "" {
		title = card.Title
	}
	b.Reminders = append(b.Reminders, Reminder{Title: title, ThreadID: body.ThreadID, At: e.now().UTC()})
	if err := e.State.Save(b); err != nil {
		return model.ActResult{Status: "failed", Summary: err.Error()}, nil
	}
	return model.ActResult{Status: "done", Summary: "Recorded the reminder. Nothing was sent."}, nil
}

type actPayload struct {
	Op        string `json:"op"`
	Provider  string `json:"provider,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	To        string `json:"to,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Body      string `json:"body,omitempty"`
	Label     string `json:"label,omitempty"`
	Title     string `json:"title,omitempty"`
}

type hit struct {
	Thread mailbox.Thread
	Rules  map[string]bool
	Labels []string
}

func (e *Engine) evaluate(ctx context.Context, since *time.Time) (Outcome, error) {
	p := e.Plugin
	now := e.now()
	blob, err := e.State.Load()
	if err != nil {
		return Outcome{}, err
	}
	blob.Runs++
	if err := e.State.Save(blob); err != nil {
		return Outcome{}, err
	}

	var threads []mailbox.Thread
	note := ""
	if p.usesEmail() {
		threads, note, err = e.loadMail(ctx, now)
		if err != nil {
			return Outcome{}, err
		}
		if p.HasEvent(EventEmail) && since != nil {
			threads = newerThan(threads, *since)
		}
	}

	windowHits, err := e.classify(threads, now)
	if err != nil {
		return Outcome{}, err
	}
	limit := e.limit()
	if len(windowHits) > limit {
		windowHits = windowHits[:limit]
	}

	title := p.Name
	if p.Output.Report != nil && strings.TrimSpace(p.Output.Report.Title) != "" {
		title = p.Output.Report.Title
	}
	var obs []model.Observation
	var lines []string
	for _, h := range windowHits {
		line := e.line(h)
		lines = append(lines, line)
		obs = append(obs, model.Observation{
			ID:      h.Thread.ID,
			At:      h.Thread.LastAt,
			Summary: line,
			Data: map[string]any{
				"from":    h.Thread.WithEmail,
				"subject": h.Thread.Subject,
			},
		})
	}
	status := model.HealthOK
	label := "Threads"
	tileValue := fmt.Sprintf("%d", len(windowHits))
	if !p.usesEmail() {
		label = "Steps"
		tileValue = fmt.Sprintf("%d", e.stepCount())
	}
	if note != "" {
		status = model.HealthDegraded
		tileValue = note
	}
	out := Outcome{
		Tile: model.Tile{
			Title:  title,
			Status: status,
			Lines:  []model.TileLine{{Label: label, Value: tileValue}},
		},
		Observations: obs,
		Note:         note,
		Metrics: []model.Metric{
			{Key: "runs", Label: "Runs", Value: blob.Runs},
		},
	}
	out.Cards = e.cards(windowHits, lines, note, title)
	return out, nil
}

func (e *Engine) loadMail(ctx context.Context, now time.Time) ([]mailbox.Thread, string, error) {
	if e.Mail == nil {
		return nil, "No mailbox is connected", nil
	}
	window, err := e.window()
	if err != nil {
		return nil, "", err
	}
	providers, err := e.Mail.Providers(ctx)
	if err != nil {
		return nil, "", err
	}
	if len(providers) == 0 {
		return nil, "No mailbox is connected", nil
	}
	var all []mailbox.Thread
	var failed []string
	for _, provider := range providers {
		threads, err := e.Mail.Threads(ctx, provider, window)
		if err != nil {
			failed = append(failed, provider)
			continue
		}
		all = append(all, threads...)
	}
	if len(failed) == len(providers) {
		return nil, "Mailbox could not be read", nil
	}
	all = within(all, window, now)
	note := ""
	if len(failed) > 0 {
		note = "Some mailboxes could not be read"
	}
	return all, note, nil
}

func (e *Engine) window() (string, error) {
	for _, in := range e.Plugin.Inputs {
		if in.Service != ServiceEmail {
			continue
		}
		if in.WindowSetting != "" {
			s, err := settingString(e.Config, in.WindowSetting)
			if err != nil {
				return "", err
			}
			if _, err := parseAge(s); err != nil {
				return "", fmt.Errorf("setting %s: %w", in.WindowSetting, err)
			}
			return s, nil
		}
		return in.Window, nil
	}
	return "", nil
}

func within(threads []mailbox.Thread, window string, now time.Time) []mailbox.Thread {
	if window == "" {
		return threads
	}
	d, err := parseAge(window)
	if err != nil {
		return threads
	}
	cut := now.Add(-d)
	out := make([]mailbox.Thread, 0, len(threads))
	for _, th := range threads {
		if th.LastAt.IsZero() || th.LastAt.Before(cut) {
			continue
		}
		out = append(out, th)
	}
	return out
}

func newerThan(threads []mailbox.Thread, since time.Time) []mailbox.Thread {
	out := make([]mailbox.Thread, 0, len(threads))
	for _, th := range threads {
		if th.LastAt.IsZero() || !th.LastAt.After(since) {
			continue
		}
		out = append(out, th)
	}
	return out
}

func (e *Engine) classify(threads []mailbox.Thread, now time.Time) ([]hit, error) {
	var out []hit
	for _, th := range threads {
		h := hit{Thread: th, Rules: map[string]bool{}}
		matched := len(e.Plugin.Rules) == 0
		for _, rule := range e.Plugin.Rules {
			ok, err := rule.match(e.Plugin, e.Config, th, now)
			if err != nil {
				return nil, err
			}
			if ok {
				matched = true
				h.Rules[rule.ID] = true
			}
		}
		if matched {
			h.Labels = e.labelsFor(h)
			out = append(out, h)
			continue
		}
		if e.Plugin.Output.UnmatchedLabel != "" {
			h.Labels = []string{e.Plugin.Output.UnmatchedLabel}
			h.Rules = nil
			out = append(out, h)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Thread.LastAt.After(out[j].Thread.LastAt)
	})
	return out, nil
}

func (e *Engine) labelsFor(h hit) []string {
	var labels []string
	for _, a := range e.Plugin.Output.Actions {
		if a.Type != "label" {
			continue
		}
		if a.Rule != "" && (h.Rules == nil || !h.Rules[a.Rule]) {
			continue
		}
		if h.Rules == nil {
			continue
		}
		labels = append(labels, a.Label)
	}
	return labels
}

func (e *Engine) limit() int {
	if e.Plugin.Output.LimitSetting != "" {
		if n, ok := asInt(e.Config[e.Plugin.Output.LimitSetting]); ok && n >= 1 && n <= 50 {
			return n
		}
	}
	if e.Plugin.Output.Limit >= 1 && e.Plugin.Output.Limit <= 50 {
		return e.Plugin.Output.Limit
	}
	return defaultLimit
}

func (e *Engine) line(h hit) string {
	kind := "subject"
	if e.Plugin.Output.Report != nil && e.Plugin.Output.Report.Line != "" {
		kind = e.Plugin.Output.Report.Line
	}
	switch kind {
	case "sender":
		if h.Thread.WithName != "" {
			return h.Thread.WithName + " <" + h.Thread.WithEmail + ">"
		}
		return h.Thread.WithEmail
	case "snippet":
		if h.Thread.Snippet != "" {
			return h.Thread.Snippet
		}
		return h.Thread.Subject
	case "label":
		if len(h.Labels) > 0 {
			return strings.Join(h.Labels, ", ")
		}
		return h.Thread.Subject
	default:
		if len(h.Labels) > 0 {
			return strings.Join(h.Labels, ", ") + ": " + orSubject(h.Thread.Subject)
		}
		return orSubject(h.Thread.Subject)
	}
}

func orSubject(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(no subject)"
	}
	return s
}

func (e *Engine) cards(hits []hit, lines []string, note, title string) []model.CardDraft {
	var cards []model.CardDraft
	if e.Plugin.Output.Report != nil {
		cards = append(cards, e.reportCard(lines, note, title))
	}
	for _, h := range hits {
		if h.Rules == nil {
			continue
		}
		for _, a := range e.Plugin.Output.Actions {
			if a.Type == "checklist" {
				continue
			}
			if a.Rule != "" && !h.Rules[a.Rule] {
				continue
			}
			if card, ok := e.actionCard(h, a); ok {
				cards = append(cards, card)
			}
		}
	}
	for _, a := range e.Plugin.Output.Actions {
		if a.Type == "checklist" {
			cards = append(cards, e.checklist(a)...)
		}
	}
	if len(cards) > 40 {
		cards = cards[:40]
	}
	return cards
}

func (e *Engine) reportCard(lines []string, note, title string) model.CardDraft {
	n := len(lines)
	heading := title + ": nothing new"
	if note != "" && n == 0 {
		heading = title
	} else if n == 1 {
		heading = title + ": 1 thread"
	} else if n > 1 {
		heading = fmt.Sprintf("%s: %d threads", title, n)
	}
	body := "No matching mail."
	if !e.Plugin.usesEmail() {
		body = "Nothing to report."
	}
	if note != "" && n == 0 {
		body = note + ". Connect Gmail, Outlook, or the demo mailbox under Connections."
	} else if n > 0 {
		shown := lines
		if len(shown) > 12 {
			shown = shown[:12]
		}
		var b strings.Builder
		for _, line := range shown {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		body = strings.TrimRight(b.String(), "\n")
	}
	payload, _ := json.Marshal(actPayload{Op: "report"})
	editable := false
	return model.CardDraft{
		Title:      heading,
		Summary:    "Report only. Nothing is sent.",
		Body:       body,
		Kind:       model.KindAction,
		Permission: "report.review",
		Effects:    []string{},
		Editable:   &editable,
		Payload:    payload,
	}
}

func (e *Engine) actionCard(h hit, a Action) (model.CardDraft, bool) {
	subject := orSubject(h.Thread.Subject)
	sig := ""
	if a.SignatureSetting != "" {
		sig, _ = settingString(e.Config, a.SignatureSetting)
	}
	text := renderBody(a.Body, subject, h.Thread.Snippet, sig)
	subj := subject
	if a.Subject != "same" {
		subj = replySubject(subject)
	}
	payload := actPayload{
		Provider:  h.Thread.Provider,
		ThreadID:  h.Thread.ID,
		MessageID: h.Thread.MessageID,
		To:        h.Thread.WithEmail,
		Subject:   subj,
		Body:      text,
	}
	switch a.Type {
	case "label":
		payload.Op = "label"
		payload.Label = a.Label
		raw, _ := json.Marshal(payload)
		editable := false
		return model.CardDraft{
			Title:      "Label " + subject + " as " + a.Label,
			Summary:    "Records a label after you approve. Nothing is sent.",
			Body:       a.Label + "\n\n" + h.Thread.Snippet,
			Kind:       model.KindAction,
			Permission: "mail.label",
			Editable:   &editable,
			Payload:    raw,
			Evidence:   []model.Evidence{{Label: "Thread", Ref: h.Thread.ID}},
		}, true
	case "draft":
		payload.Op = "draft"
		raw, _ := json.Marshal(payload)
		editable := true
		return model.CardDraft{
			Title:      "Draft: " + subject,
			Summary:    "A draft is prepared when you approve. Nothing is sent.",
			Body:       text,
			Kind:       model.KindAction,
			Permission: "mail.draft",
			Editable:   &editable,
			Payload:    raw,
			Evidence:   []model.Evidence{{Label: "Thread", Ref: h.Thread.ID}},
		}, true
	case "reminder":
		title := a.Title
		if title == "" {
			title = "Reminder: " + subject
		}
		payload.Op = "reminder"
		payload.Title = title
		raw, _ := json.Marshal(payload)
		editable := true
		return model.CardDraft{
			Title:      title,
			Summary:    "Records a reminder when you approve. Nothing is sent.",
			Body:       subject + "\n" + h.Thread.WithEmail,
			Kind:       model.KindAction,
			Permission: "reminder.create",
			Editable:   &editable,
			Payload:    raw,
			Evidence:   []model.Evidence{{Label: "Thread", Ref: h.Thread.ID}},
		}, true
	case "send":
		payload.Op = "send"
		raw, _ := json.Marshal(payload)
		editable := true
		return model.CardDraft{
			Title:      "Send: " + subject,
			Summary:    "Sends only after you approve.",
			Body:       text,
			Kind:       model.KindAction,
			Permission: "mail.send",
			Effects:    []string{model.EffectSends},
			Editable:   &editable,
			Payload:    raw,
			Evidence:   []model.Evidence{{Label: "Thread", Ref: h.Thread.ID}},
		}, true
	default:
		return model.CardDraft{}, false
	}
}

func (e *Engine) stepCount() int {
	n := 0
	for _, a := range e.Plugin.Output.Actions {
		if a.Type != "checklist" {
			continue
		}
		n += len(e.checklist(a))
	}
	return n
}

func (e *Engine) checklist(a Action) []model.CardDraft {
	raw, _ := json.Marshal(e.Config[a.StepsSetting])
	var steps []step
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil
	}
	var cards []model.CardDraft
	for _, s := range steps {
		if strings.TrimSpace(s.Title) == "" {
			continue
		}
		effects := normalizeEffects(s.Effects)
		summary := "This step does not send, spend, or delete. It still waits for approval."
		if len(effects) > 0 {
			summary = "This step waits for approval."
		}
		payload, _ := json.Marshal(actPayload{Op: "checklist", Title: s.Title})
		editable := len(effects) > 0
		body := s.Detail
		if body == "" {
			body = s.Title
		}
		cards = append(cards, model.CardDraft{
			Title:      s.Title,
			Summary:    summary,
			Body:       body,
			Kind:       model.KindAction,
			Permission: "workflow.run",
			Effects:    effects,
			Editable:   &editable,
			Payload:    payload,
		})
	}
	return cards
}

type step struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Detail  string   `json:"detail"`
	Effects []string `json:"effects"`
}

func normalizeEffects(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.ToLower(strings.TrimSpace(e))
		if !effectOK(e) || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

func renderBody(style, subject, snippet, signature string) string {
	switch style {
	case "nudge":
		b := "Checking in on " + subject + ".\n"
		if strings.TrimSpace(signature) != "" {
			b += "\n" + signature + "\n"
		}
		return b
	case "snippet":
		heard := ""
		snip := snippet
		if len(snip) > 180 {
			snip = snip[:180]
		}
		if strings.TrimSpace(snip) != "" {
			heard = " I saw: " + snip
		}
		return "Thanks for the note about " + subject + "." + heard + "\n\nI will follow up shortly.\n"
	default:
		return "Thanks for the note about " + subject + ".\n\nI will follow up shortly.\n"
	}
}

func replySubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		return subject
	}
	return "Re: " + subject
}

// Sample is the install-card preview. threads is the demo mailbox.
// Nothing is sent or saved.
func Sample(p Plugin, given map[string]any, now time.Time, threads []mailbox.Thread) (Outcome, string, error) {
	cfg, err := p.Resolve(given)
	if err != nil {
		return Outcome{}, "", err
	}
	eng := New(p, cfg)
	eng.Now = func() time.Time { return now }
	eng.Mail = staticMail{threads: threads}
	eng.State = &Memory{}
	out, err := eng.Watch(context.Background(), nil)
	if err != nil {
		return Outcome{}, "", err
	}
	return out, formatSample(p, out), nil
}

func formatSample(p Plugin, out Outcome) string {
	var b strings.Builder
	if p.usesEmail() {
		b.WriteString("Sample run (demo mailbox). Nothing is sent or saved.\n")
	} else {
		b.WriteString("Sample run. Nothing is sent or saved.\n")
	}
	if p.PollsEvent() {
		b.WriteString("This event is polled every " + EventPoll + " by the core. Uninstall removes that poll.\n")
	}
	if out.Note != "" {
		b.WriteString(out.Note + "\n")
	}
	if len(out.Cards) == 0 {
		b.WriteString("No cards.\n")
		return b.String()
	}
	b.WriteString("Cards:\n")
	n := len(out.Cards)
	if n > 8 {
		n = 8
	}
	for i := 0; i < n; i++ {
		c := out.Cards[i]
		fmt.Fprintf(&b, "- %s\n", c.Title)
	}
	if len(out.Cards) > n {
		fmt.Fprintf(&b, "- … %d more\n", len(out.Cards)-n)
	}
	return b.String()
}

type staticMail struct{ threads []mailbox.Thread }

func (s staticMail) Providers(context.Context) ([]string, error) {
	return []string{"fake"}, nil
}
func (s staticMail) Threads(context.Context, string, string) ([]mailbox.Thread, error) {
	return s.threads, nil
}
func (s staticMail) Draft(context.Context, mailbox.SendRequest) (mailbox.DraftResult, error) {
	return mailbox.DraftResult{Provider: "fake", Local: true}, nil
}
func (s staticMail) Send(context.Context, mailbox.SendRequest) (mailbox.SendResult, error) {
	return mailbox.SendResult{Provider: "fake"}, nil
}
