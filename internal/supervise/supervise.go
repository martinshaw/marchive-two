// Package supervise installs the per-user login service that starts Marchive.
// Linux uses a systemd user unit, macOS a LaunchAgent, and Windows Task Scheduler.
package supervise

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/martinshaw/marchive-two/internal/paths"
)

const (
	systemdName = "marchive.service"
	launchLabel = "co.martinshaw.marchive"
	taskName    = "Marchive"
)

// Spec is the login service to install for one data directory.
type Spec struct {
	Executable     string
	DataDir        string
	OpenOnLogin    bool
	RestartOnCrash bool
}

// Current reports the login service installed for this user.
func Current() (openOnLogin bool, restartOnCrash bool, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, false, err
	}
	return Read(runtime.GOOS, home)
}

// Apply writes the login service and enables or removes it.
func Apply(spec Spec) error {
	if strings.TrimSpace(spec.DataDir) == "" {
		return errors.New("data directory is missing")
	}
	if spec.Executable == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		spec.Executable = exe
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	steps, err := Plan(spec, runtime.GOOS, home, os.Getuid())
	if err != nil {
		return err
	}
	return execute(steps)
}

// Read interprets the login service files under home.
func Read(goos, home string) (bool, bool, error) {
	switch goos {
	case "linux":
		body, err := os.ReadFile(systemdUnitPath(home))
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		_, err = os.Lstat(systemdEnablePath(home))
		open := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, false, err
		}
		return open, strings.Contains(string(body), "\nRestart=on-failure\n"), nil
	case "darwin":
		body, err := os.ReadFile(launchdPath(home))
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		text := string(body)
		return strings.Contains(text, "<key>RunAtLoad</key>\n    <true/>"),
			strings.Contains(text, "<key>SuccessfulExit</key>"), nil
	case "windows":
		text, err := readText(taskPath(home))
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		return strings.Contains(text, "<LogonTrigger>"),
			strings.Contains(text, "<RestartOnFailure>"), nil
	default:
		return false, false, nil
	}
}

type step struct {
	path     string
	body     []byte
	remove   bool
	cmd      string
	args     []string
	optional bool
	ignore   []string
}

// Plan is the files and commands that install or remove the login service.
func Plan(spec Spec, goos, home string, uid int) ([]step, error) {
	if strings.TrimSpace(spec.Executable) == "" || strings.TrimSpace(spec.DataDir) == "" {
		return nil, errors.New("executable and data directory are required")
	}
	switch goos {
	case "linux":
		return planLinux(spec, home), nil
	case "darwin":
		return planDarwin(spec, home, uid), nil
	case "windows":
		return planWindows(spec, home), nil
	default:
		return nil, fmt.Errorf("%s has no login service", goos)
	}
}

func planLinux(spec Spec, home string) []step {
	unit := systemdUnitPath(home)
	if !spec.OpenOnLogin && !spec.RestartOnCrash {
		return []step{
			{cmd: "systemctl", args: []string{"--user", "disable", systemdName}, optional: true},
			{path: unit, remove: true},
			{cmd: "systemctl", args: []string{"--user", "daemon-reload"}, optional: true},
		}
	}
	steps := []step{
		{path: unit, body: []byte(systemdUnit(spec))},
		{cmd: "systemctl", args: []string{"--user", "daemon-reload"}},
	}
	if spec.OpenOnLogin {
		steps = append(steps, step{cmd: "systemctl", args: []string{"--user", "enable", systemdName}})
	} else {
		steps = append(steps, step{cmd: "systemctl", args: []string{"--user", "disable", systemdName}, optional: true})
	}
	return steps
}

func planDarwin(spec Spec, home string, uid int) []step {
	plist := launchdPath(home)
	target := fmt.Sprintf("gui/%d/%s", uid, launchLabel)
	domain := fmt.Sprintf("gui/%d", uid)
	if !spec.OpenOnLogin && !spec.RestartOnCrash {
		return []step{
			{cmd: "launchctl", args: []string{"bootout", target}, optional: true},
			{path: plist, remove: true},
		}
	}
	return []step{
		{path: plist, body: []byte(launchdPlist(spec))},
		{cmd: "launchctl", args: []string{"bootstrap", domain, plist}, ignore: []string{"already", "File exists", "Input/output error"}},
		{cmd: "launchctl", args: []string{enableWord(spec.OpenOnLogin), target}},
	}
}

func planWindows(spec Spec, home string) []step {
	xmlPath := taskPath(home)
	if !spec.OpenOnLogin && !spec.RestartOnCrash {
		return []step{
			{cmd: "schtasks", args: []string{"/Delete", "/TN", taskName, "/F"}, optional: true},
			{path: xmlPath, remove: true},
		}
	}
	return []step{
		{path: xmlPath, body: utf16LE(windowsTask(spec))},
		{cmd: "schtasks", args: []string{"/Create", "/TN", taskName, "/XML", xmlPath, "/F"}},
	}
}

func enableWord(on bool) string {
	if on {
		return "enable"
	}
	return "disable"
}

func systemdUnitPath(home string) string {
	return filepath.Join(home, ".config", "systemd", "user", systemdName)
}

func systemdEnablePath(home string) string {
	return filepath.Join(home, ".config", "systemd", "user", "default.target.wants", systemdName)
}

func launchdPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist")
}

func taskPath(home string) string {
	return filepath.Join(home, "AppData", "Roaming", "Marchive", "login-task.xml")
}

func systemdUnit(spec Spec) string {
	restart := "no"
	if spec.RestartOnCrash {
		restart = "on-failure"
	}
	log := paths.Log(spec.DataDir)
	return "[Unit]\n" +
		"Description=Marchive\n" +
		"After=network-online.target\n" +
		"\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=" + strconv.Quote(spec.Executable) + " serve --data-dir " + strconv.Quote(spec.DataDir) + "\n" +
		"WorkingDirectory=" + strconv.Quote(spec.DataDir) + "\n" +
		"Restart=" + restart + "\n" +
		"RestartSec=5\n" +
		"StartLimitIntervalSec=300\n" +
		"StartLimitBurst=10\n" +
		"StandardOutput=append:" + log + "\n" +
		"StandardError=append:" + log + "\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}

func launchdPlist(spec Spec) string {
	log := xmlText(paths.Log(spec.DataDir))
	keep := ""
	if spec.RestartOnCrash {
		keep = "    <key>KeepAlive</key>\n" +
			"    <dict>\n" +
			"        <key>SuccessfulExit</key>\n" +
			"        <false/>\n" +
			"    </dict>\n" +
			"    <key>ThrottleInterval</key>\n" +
			"    <integer>5</integer>\n"
	}
	runAtLoad := "<false/>"
	if spec.OpenOnLogin {
		runAtLoad = "<true/>"
	}
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n" +
		"<plist version=\"1.0\">\n" +
		"<dict>\n" +
		"    <key>Label</key>\n" +
		"    <string>" + launchLabel + "</string>\n" +
		"    <key>ProgramArguments</key>\n" +
		"    <array>\n" +
		"        <string>" + xmlText(spec.Executable) + "</string>\n" +
		"        <string>serve</string>\n" +
		"        <string>--data-dir</string>\n" +
		"        <string>" + xmlText(spec.DataDir) + "</string>\n" +
		"    </array>\n" +
		"    <key>WorkingDirectory</key>\n" +
		"    <string>" + xmlText(spec.DataDir) + "</string>\n" +
		"    <key>RunAtLoad</key>\n" +
		"    " + runAtLoad + "\n" +
		keep +
		"    <key>StandardOutPath</key>\n" +
		"    <string>" + log + "</string>\n" +
		"    <key>StandardErrorPath</key>\n" +
		"    <string>" + log + "</string>\n" +
		"</dict>\n" +
		"</plist>\n"
}

func windowsTask(spec Spec) string {
	trigger := ""
	if spec.OpenOnLogin {
		trigger = "    <Triggers>\n      <LogonTrigger><Enabled>true</Enabled></LogonTrigger>\n    </Triggers>\n"
	}
	restart := ""
	if spec.RestartOnCrash {
		restart = "      <RestartOnFailure>\n        <Interval>PT1M</Interval>\n        <Count>999</Count>\n      </RestartOnFailure>\n"
	}
	return "<?xml version=\"1.0\" encoding=\"UTF-16\"?>\n" +
		"<Task version=\"1.4\" xmlns=\"http://schemas.microsoft.com/windows/2004/02/mit/task\">\n" +
		trigger +
		"  <Settings>\n" +
		"    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>\n" +
		"    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>\n" +
		"    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>\n" +
		"    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>\n" +
		"    <Enabled>true</Enabled>\n" +
		restart +
		"  </Settings>\n" +
		"  <Actions>\n" +
		"    <Exec>\n" +
		"      <Command>" + xmlText(spec.Executable) + "</Command>\n" +
		"      <Arguments>serve --data-dir " + xmlText(strconv.Quote(spec.DataDir)) + "</Arguments>\n" +
		"      <WorkingDirectory>" + xmlText(spec.DataDir) + "</WorkingDirectory>\n" +
		"    </Exec>\n" +
		"  </Actions>\n" +
		"</Task>\n"
}

func utf16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	out := make([]byte, 2+len(units)*2)
	out[0], out[1] = 0xFF, 0xFE
	for i, u := range units {
		out[2+i*2] = byte(u)
		out[3+i*2] = byte(u >> 8)
	}
	return out
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		units := make([]uint16, (len(b)-2)/2)
		for i := range units {
			units[i] = uint16(b[2+i*2]) | uint16(b[3+i*2])<<8
		}
		return string(utf16.Decode(units)), nil
	}
	return string(b), nil
}

func xmlText(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return replacer.Replace(s)
}

func execute(steps []step) error {
	for _, s := range steps {
		switch {
		case s.body != nil:
			if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(s.path, s.body, 0o644); err != nil {
				return err
			}
		case s.remove:
			if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		case s.cmd != "":
			if _, err := exec.LookPath(s.cmd); err != nil {
				if s.optional {
					continue
				}
				return fmt.Errorf("%s is not available on this computer", s.cmd)
			}
			out, err := exec.Command(s.cmd, s.args...).CombinedOutput()
			if err != nil && !ignored(s, out) {
				msg := strings.TrimSpace(string(out))
				if msg == "" {
					msg = err.Error()
				}
				return fmt.Errorf("could not update the login service: %s", msg)
			}
		}
	}
	return nil
}

func ignored(s step, out []byte) bool {
	if s.optional {
		return true
	}
	text := string(out)
	for _, needle := range s.ignore {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
