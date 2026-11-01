package permissions

import (
	"strings"
	"testing"

	"github.com/vicgupta/dap/internal/manifest"
	"github.com/vicgupta/dap/internal/model"
)

func TestLookupAndPlain(t *testing.T) {
	declared := []manifest.Permission{
		{Name: "mail.send", Mode: "ask first", Description: "Send mail as you"},
		{Name: "web.read", Mode: model.ModeAllowed},
	}
	d := Lookup(declared, "files.write")
	if d.Declared || d.Mode != model.ModeNever {
		t.Fatalf("%+v", d)
	}
	if !strings.Contains(d.Plain, "denied") {
		t.Fatal(d.Plain)
	}
	d = Lookup(declared, "mail.send")
	if d.Mode != model.ModeAskFirst || !strings.Contains(d.Plain, "Send mail as you") {
		t.Fatalf("%+v", d)
	}
	d = Lookup(declared, "web.read")
	if d.Mode != model.ModeAllowed || !strings.Contains(d.Plain, "Read public web pages") {
		t.Fatalf("%+v", d)
	}
	if Lookup(declared, "").Mode != model.ModeAllowed {
		t.Fatal("empty permission should be allowed")
	}
}
