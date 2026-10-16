package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeArchive(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := HomeArchive()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Marchive")
	if got != want {
		t.Fatalf("HomeArchive() = %q, want %q", got, want)
	}
}
