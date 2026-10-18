package cli

import (
	"strings"
	"testing"
)

func TestReadSetupDefaultsDaemonSettingsOn(t *testing.T) {
	in := strings.NewReader("8080\n\n\n")
	var out strings.Builder
	port, open, restart, err := readSetup(in, &out, 0)
	if err != nil {
		t.Fatal(err)
	}
	if port != 8080 || !open || !restart {
		t.Fatalf("port %d open %v restart %v", port, open, restart)
	}
	if !strings.Contains(out.String(), "Open on login? [Y/n]") || !strings.Contains(out.String(), "Restart on crash? [Y/n]") {
		t.Fatalf("prompts %q", out.String())
	}
}

func TestReadSetupKeepsCurrentPortAndAcceptsNo(t *testing.T) {
	in := strings.NewReader("\nno\nN\n")
	var out strings.Builder
	port, open, restart, err := readSetup(in, &out, 9090)
	if err != nil {
		t.Fatal(err)
	}
	if port != 9090 || open || restart {
		t.Fatalf("port %d open %v restart %v", port, open, restart)
	}
	if !strings.Contains(out.String(), "[9090]") {
		t.Fatalf("prompts %q", out.String())
	}
}

func TestReadSetupRetriesBadAnswers(t *testing.T) {
	in := strings.NewReader("0\n70000\n57067\nmaybe\nyes\nnope\nn\n")
	port, open, restart, err := readSetup(in, &strings.Builder{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if port != 57067 || !open || restart {
		t.Fatalf("port %d open %v restart %v", port, open, restart)
	}
}

func TestReadSetupCancelledOnEOF(t *testing.T) {
	_, _, _, err := readSetup(strings.NewReader(""), &strings.Builder{}, 0)
	if err == nil || err.Error() != "setup cancelled" {
		t.Fatalf("err %v", err)
	}
}
