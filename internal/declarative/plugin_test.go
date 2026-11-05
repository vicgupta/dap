package declarative

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileInboxTemplate(t *testing.T) {
	raw := readRepo(t, "templates/inbox-triage/plugin.yaml")
	p, err := Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "inbox-triage" || !p.HasEvent(EventEmail) {
		t.Fatalf("%+v", p)
	}
	perms := map[string]string{}
	for _, perm := range p.Permissions() {
		perms[perm.Name] = perm.Mode
	}
	if perms["mail.read"] != ModeAllowed || perms["mail.label"] != ModeAskFirst || perms["report.review"] != ModeAskFirst {
		t.Fatalf("%v", perms)
	}
	if _, ok := perms["mail.send"]; ok {
		t.Fatal("send was not declared by the actions")
	}
}

func TestSchemaAndSemanticErrors(t *testing.T) {
	digest := string(readRepo(t, "templates/daily-digest/plugin.yaml"))
	nudger := string(readRepo(t, "templates/follow-up-nudger/plugin.yaml"))
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"service", strings.Replace(digest, "service: email", "service: calendar", 1), "email"},
		{"llm", strings.Replace(digest, "kind: declarative", "kind: declarative\nllm:\n  task: classify", 1), "not supported"},
		{"script", strings.Replace(nudger, "last_from_me: true", "expr: 1 + 1", 1), "expr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compile([]byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestCodePluginIsNotDeclarative(t *testing.T) {
	raw := readRepo(t, "plugins/hello-watch/plugin.yaml")
	if LooksLike(raw) {
		t.Fatal("hello-watch looked declarative")
	}
}

func TestPermissionsFollowActions(t *testing.T) {
	raw := readRepo(t, "templates/scheduled-checklist/plugin.yaml")
	p, err := Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	perms := p.Permissions()
	if len(perms) != 1 || perms[0].Name != "workflow.run" || perms[0].Mode != ModeAskFirst {
		t.Fatalf("%+v", perms)
	}
	if !p.EffectiveSchedule().Empty() && p.EffectiveSchedule().Every != "1h" {
		t.Fatal(p.EffectiveSchedule())
	}
}

func TestEventOnlyPolls(t *testing.T) {
	raw := []byte(`
apiVersion: dap/v1
kind: declarative
name: new-mail
version: 0.1.0
min_dap_version: 0.1.0
triggers:
  - event: email.received
inputs:
  - service: email
    window: 1h
output:
  report:
    title: New mail
`)
	p, err := Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !p.PollsEvent() || p.EffectiveSchedule().Every != EventPoll {
		t.Fatalf("%+v", p.EffectiveSchedule())
	}
}

func readRepo(t *testing.T, rel string) []byte {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			b, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root")
		}
		dir = parent
	}
}
