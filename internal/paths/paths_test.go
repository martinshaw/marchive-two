package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirFor(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "Users", "ada")
	if dataDirFor("darwin", home, "", "") != filepath.Join(home, "Library", "Application Support", "Marchive") {
		t.Fatal("darwin")
	}
	if dataDirFor("windows", home, `C:\Users\ada\AppData\Roaming`, "") != filepath.Join(`C:\Users\ada\AppData\Roaming`, "Marchive") {
		t.Fatal("windows")
	}
	if dataDirFor("linux", home, "", "/var/lib/data") != filepath.Join("/var/lib/data", "Marchive") {
		t.Fatal("xdg")
	}
	if dataDirFor("linux", home, "", "") != filepath.Join(home, ".local", "share", "Marchive") {
		t.Fatal("linux")
	}
}

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
