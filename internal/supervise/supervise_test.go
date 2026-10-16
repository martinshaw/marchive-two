package supervise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanLinuxLoginAndRestart(t *testing.T) {
	home := t.TempDir()
	spec := Spec{Executable: "/usr/local/bin/marchive", DataDir: "/data/Marchive", OpenOnLogin: true, RestartOnCrash: true}
	steps, err := Plan(spec, "linux", home, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFiles(steps); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(systemdEnablePath(home)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(systemdUnitPath(home), systemdEnablePath(home)); err != nil {
		t.Fatal(err)
	}
	open, restart, err := Read("linux", home)
	if err != nil || !open || !restart {
		t.Fatalf("state %v %v %v", open, restart, err)
	}
	body, err := os.ReadFile(systemdUnitPath(home))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `ExecStart="/usr/local/bin/marchive" serve --data-dir "/data/Marchive"`) {
		t.Fatalf("unit:\n%s", text)
	}
	if !hasCommand(steps, "systemctl", "--user", "enable", systemdName) {
		t.Fatalf("steps %+v", steps)
	}
}

func TestPlanRemovesServiceWhenBothOff(t *testing.T) {
	home := t.TempDir()
	spec := Spec{Executable: "/bin/marchive", DataDir: "/data", OpenOnLogin: true, RestartOnCrash: false}
	steps, err := Plan(spec, "darwin", home, 501)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFiles(steps); err != nil {
		t.Fatal(err)
	}
	open, restart, err := Read("darwin", home)
	if err != nil || !open || restart {
		t.Fatalf("installed %v %v %v", open, restart, err)
	}
	off, err := Plan(Spec{Executable: "/bin/marchive", DataDir: "/data"}, "darwin", home, 501)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFiles(off); err != nil {
		t.Fatal(err)
	}
	open, restart, err = Read("darwin", home)
	if err != nil || open || restart {
		t.Fatalf("removed %v %v %v", open, restart, err)
	}
	if !hasCommand(off, "launchctl", "bootout", "gui/501/"+launchLabel) {
		t.Fatalf("bootout steps %+v", off)
	}
}

func TestPlanWindowsRestartWithoutLogin(t *testing.T) {
	home := t.TempDir()
	spec := Spec{Executable: `C:\Marchive\marchive.exe`, DataDir: `C:\Users\Ada\Marchive`, RestartOnCrash: true}
	steps, err := Plan(spec, "windows", home, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFiles(steps); err != nil {
		t.Fatal(err)
	}
	open, restart, err := Read("windows", home)
	if err != nil || open || !restart {
		t.Fatalf("state %v %v %v", open, restart, err)
	}
	text, err := readText(taskPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "<Count>999</Count>") || strings.Contains(text, "<LogonTrigger>") {
		t.Fatalf("task:\n%s", text)
	}
}

func TestReadMissing(t *testing.T) {
	home := t.TempDir()
	for _, goos := range []string{"linux", "darwin", "windows"} {
		open, restart, err := Read(goos, home)
		if err != nil || open || restart {
			t.Fatalf("%s %v %v %v", goos, open, restart, err)
		}
	}
}

func writeFiles(steps []step) error {
	for _, s := range steps {
		if s.body == nil && !s.remove {
			continue
		}
		if s.remove {
			if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(s.path, s.body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func hasCommand(steps []step, name string, args ...string) bool {
	for _, s := range steps {
		if s.cmd != name || len(s.args) != len(args) {
			continue
		}
		match := true
		for i := range args {
			if s.args[i] != args[i] {
				match = false
			}
		}
		if match {
			return true
		}
	}
	return false
}
