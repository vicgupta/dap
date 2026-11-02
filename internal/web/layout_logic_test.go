package web

import (
	"os/exec"
	"testing"
)

func TestLayoutLogic(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	cmd := exec.Command("node", "layout-logic.test.js")
	cmd.Dir = "static"
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
