// Package service is the API the UI and the command line share.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/paths"
	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/runner"
	"github.com/martinshaw/marchive-two/internal/store"
	"github.com/martinshaw/marchive-two/internal/supervise"
	"github.com/martinshaw/marchive-two/internal/ytdlp"
)

// Error is a failure with an HTTP status.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

// SourceInput creates a source and its first schedule.
type SourceInput struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	Provider         string `json:"provider"`
	LinkSelector     string `json:"linkSelector"`
	Capture          string `json:"capture"`
	IntervalSeconds  int    `json:"intervalSeconds"`
	DownloadLocation string `json:"downloadLocation"`
	MaxNew           int    `json:"maxNew"`
	Enabled          *bool  `json:"enabled"`
}

// ScheduleInput updates the fields that are set.
type ScheduleInput struct {
	IntervalSeconds  *int    `json:"intervalSeconds"`
	DownloadLocation *string `json:"downloadLocation"`
	MaxNew           *int    `json:"maxNew"`
	Enabled          *bool   `json:"enabled"`
	Capture          *string `json:"capture"`
	LinkSelector     *string `json:"linkSelector"`
}

// SettingsInput changes concurrency, time limits, attempt counts, the UI port, and the login service.
// ListenPort zero means a random port on the next start.
// A nil field leaves that setting unchanged.
// ItemTimeoutSeconds and CaptureTimeoutSeconds use zero for no time limit.
type SettingsInput struct {
	PodcastConcurrency    *int  `json:"podcastConcurrency"`
	BrowserConcurrency    *int  `json:"browserConcurrency"`
	ListenPort            *int  `json:"listenPort"`
	ItemTimeoutSeconds    *int  `json:"itemTimeoutSeconds"`
	CaptureTimeoutSeconds *int  `json:"captureTimeoutSeconds"`
	ItemTries             *int  `json:"itemTries"`
	CaptureTries          *int  `json:"captureTries"`
	OpenOnLogin           *bool `json:"openOnLogin"`
	RestartOnCrash        *bool `json:"restartOnCrash"`
}

// State is the snapshot rendered by the UI.
type State struct {
	RunnerEnabled         bool          `json:"runnerEnabled"`
	SigningIn             bool          `json:"signingIn"`
	BrowserBusy           bool          `json:"browserBusy"`
	ChromePath            string        `json:"chromePath"`
	ChromeError           string        `json:"chromeError"`
	PodcastConcurrency    int           `json:"podcastConcurrency"`
	BrowserConcurrency    int           `json:"browserConcurrency"`
	ListenPort            int           `json:"listenPort"`
	ItemTimeoutSeconds    int           `json:"itemTimeoutSeconds"`
	CaptureTimeoutSeconds int           `json:"captureTimeoutSeconds"`
	ItemTries             int           `json:"itemTries"`
	CaptureTries          int           `json:"captureTries"`
	ActivePort            int           `json:"activePort"`
	OpenOnLogin           bool          `json:"openOnLogin"`
	RestartOnCrash        bool          `json:"restartOnCrash"`
	YtDlpPath             string        `json:"ytDlpPath"`
	YtDlpVersion          string        `json:"ytDlpVersion"`
	DefaultFolder         string        `json:"defaultFolder"`
	Sources               []SourceState `json:"sources"`
	Runs                  []RunState    `json:"runs"`
	Error                 string        `json:"error,omitempty"`
}

// SourceState is a source plus its schedules.
type SourceState struct {
	store.Source
	Schedules []store.Schedule `json:"schedules"`
}

// RunState is a run plus the items it produced.
type RunState struct {
	store.RunDetail
	Items []store.Item `json:"items"`
}

// Service validates requests and talks to the store and runner.
type Service struct {
	Store   *store.Store
	Runner  *runner.Runner
	Browser *browser.Client
	// ActivePort is the port this process is listening on.
	ActivePort int
	// DataDir is the directory the login service should keep using.
	DataDir string
	ytdlp   *ytdlp.Installer
}

func New(st *store.Store, run *runner.Runner, br *browser.Client) *Service {
	return &Service{Store: st, Runner: run, Browser: br, ytdlp: ytdlp.NewInstaller()}
}

func (s *Service) State() (State, error) {
	st := State{BrowserConcurrency: 1, PodcastConcurrency: 2}
	if folder, ferr := paths.HomeArchive(); ferr == nil {
		st.DefaultFolder = folder
	}
	var err error
	st.RunnerEnabled, err = s.Store.RunnerEnabled()
	if err != nil {
		return State{}, err
	}
	st.PodcastConcurrency, err = s.Store.PodcastConcurrency()
	if err != nil {
		return State{}, err
	}
	st.ListenPort, err = s.Store.ListenPort()
	if err != nil {
		return State{}, err
	}
	st.ItemTimeoutSeconds, err = s.Store.ItemTimeoutSeconds()
	if err != nil {
		return State{}, err
	}
	st.CaptureTimeoutSeconds, err = s.Store.CaptureTimeoutSeconds()
	if err != nil {
		return State{}, err
	}
	st.ItemTries, err = s.Store.ItemTries()
	if err != nil {
		return State{}, err
	}
	st.CaptureTries, err = s.Store.CaptureTries()
	if err != nil {
		return State{}, err
	}
	st.ActivePort = s.ActivePort
	st.OpenOnLogin, st.RestartOnCrash, err = supervise.Current()
	if err != nil {
		return State{}, err
	}
	if path, ferr := browser.Find(); ferr != nil {
		st.ChromeError = ferr.Error()
	} else {
		st.ChromePath = path
	}
	if s.Browser != nil {
		st.BrowserBusy, st.SigningIn = s.Browser.Status()
	}
	st.YtDlpPath, st.YtDlpVersion = ytdlp.Status(s.DataDir)
	sources, err := s.Store.ListSources()
	if err != nil {
		return State{}, err
	}
	schedules, err := s.Store.ListSchedules()
	if err != nil {
		return State{}, err
	}
	bySource := map[int64][]store.Schedule{}
	for _, detail := range schedules {
		bySource[detail.SourceID] = append(bySource[detail.SourceID], detail.Schedule)
	}
	for _, src := range sources {
		st.Sources = append(st.Sources, SourceState{Source: src, Schedules: bySource[src.ID]})
	}
	runs, err := s.Store.ListRunDetails(40)
	if err != nil {
		return State{}, err
	}
	ids := make([]int64, len(runs))
	for i := range runs {
		ids[i] = runs[i].ID
	}
	items, err := s.Store.ListItemsByRunIDs(ids)
	if err != nil {
		return State{}, err
	}
	grouped := map[int64][]store.Item{}
	for _, item := range items {
		grouped[item.RunID] = append(grouped[item.RunID], item)
	}
	for _, run := range runs {
		list := grouped[run.ID]
		if list == nil {
			list = []store.Item{}
		}
		st.Runs = append(st.Runs, RunState{RunDetail: run, Items: list})
	}
	if st.Sources == nil {
		st.Sources = []SourceState{}
	}
	if st.Runs == nil {
		st.Runs = []RunState{}
	}
	return st, nil
}

func (s *Service) CreateSource(in SourceInput) (int64, int64, error) {
	normalized, err := provider.NormalizeURL(in.URL)
	if err != nil {
		return 0, 0, &Error{Status: 400, Msg: err.Error()}
	}
	if !provider.ValidKind(in.Provider) {
		return 0, 0, &Error{Status: 400, Msg: provider.KindHint()}
	}
	if err := provider.ValidateSelector(in.LinkSelector); err != nil {
		return 0, 0, &Error{Status: 400, Msg: "link selector is not valid CSS"}
	}
	if in.Provider != provider.KindArticles {
		in.LinkSelector = ""
	}
	capture, err := provider.NormalizeStored(in.Provider, in.Capture)
	if err != nil {
		return 0, 0, &Error{Status: 400, Msg: err.Error()}
	}
	if in.IntervalSeconds != 0 && in.IntervalSeconds < 30 {
		return 0, 0, &Error{Status: 400, Msg: "the interval must be at least 30 seconds"}
	}
	if in.MaxNew < 0 || in.MaxNew > 1000 {
		return 0, 0, &Error{Status: 400, Msg: "new items per run must be between 0 and 1000"}
	}
	dest, err := cleanDest(in.DownloadLocation)
	if err != nil {
		return 0, 0, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		if u, perr := url.Parse(normalized); perr == nil {
			name = u.Hostname()
		}
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	sourceID, scheduleID, err := s.Store.CreateSourceWithSchedule(store.Source{
		Name:         name,
		URL:          normalized,
		Provider:     in.Provider,
		LinkSelector: strings.TrimSpace(in.LinkSelector),
		Capture:      capture,
	}, store.Schedule{
		IntervalSeconds:  in.IntervalSeconds,
		DownloadLocation: dest,
		Enabled:          enabled,
		MaxNew:           in.MaxNew,
	})
	if err != nil {
		return 0, 0, err
	}
	s.kick()
	return sourceID, scheduleID, nil
}

// SourceCheck is the result of asking whether an address suits a source kind.
type SourceCheck struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// SetItemsRead marks the given items read or unread.
func (s *Service) SetItemsRead(ids []int64, read bool) error {
	if len(ids) == 0 || len(ids) > 500 {
		return &Error{Status: 400, Msg: "choose an item"}
	}
	seen := map[int64]bool{}
	clean := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id < 1 || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return &Error{Status: 400, Msg: "choose an item"}
	}
	return s.Store.SetItemsRead(clean, read)
}

// CheckSource fetches rawURL and reports whether it can be captured as kind.
func (s *Service) CheckSource(ctx context.Context, rawURL, kind string) (SourceCheck, error) {
	normalized, err := provider.NormalizeURL(rawURL)
	if err != nil {
		return SourceCheck{}, &Error{Status: 400, Msg: err.Error()}
	}
	if !provider.ValidKind(kind) {
		return SourceCheck{}, &Error{Status: 400, Msg: provider.KindHint()}
	}
	title, err := provider.CheckAddress(ctx, kind, normalized)
	if err != nil {
		return SourceCheck{}, &Error{Status: 400, Msg: err.Error()}
	}
	return SourceCheck{URL: normalized, Title: title}, nil
}

func (s *Service) UpdateSchedule(id int64, in ScheduleInput) error {
	current, err := s.Store.GetSchedule(id)
	if err != nil {
		return err
	}
	if in.IntervalSeconds != nil {
		if *in.IntervalSeconds != 0 && *in.IntervalSeconds < 30 {
			return &Error{Status: 400, Msg: "the interval must be at least 30 seconds"}
		}
		current.IntervalSeconds = *in.IntervalSeconds
	}
	if in.MaxNew != nil {
		if *in.MaxNew < 0 || *in.MaxNew > 1000 {
			return &Error{Status: 400, Msg: "new items per run must be between 0 and 1000"}
		}
		current.MaxNew = *in.MaxNew
	}
	if in.Enabled != nil {
		current.Enabled = *in.Enabled
	}
	if in.DownloadLocation != nil {
		dest, err := cleanDest(*in.DownloadLocation)
		if err != nil {
			return err
		}
		current.DownloadLocation = dest
	}
	if in.Capture != nil {
		capture, err := provider.NormalizeStored(current.Source.Provider, *in.Capture)
		if err != nil {
			return &Error{Status: 400, Msg: err.Error()}
		}
		if err := s.Store.SetSourceCapture(current.SourceID, capture); err != nil {
			return err
		}
	}
	if in.LinkSelector != nil && current.Source.Provider == provider.KindArticles {
		selector := strings.TrimSpace(*in.LinkSelector)
		if err := provider.ValidateSelector(selector); err != nil {
			return &Error{Status: 400, Msg: "link selector is not valid CSS"}
		}
		if err := s.Store.SetSourceLinkSelector(current.SourceID, selector); err != nil {
			return err
		}
	}
	if err := s.Store.UpdateSchedule(current.Schedule); err != nil {
		return err
	}
	s.kick()
	return nil
}

func (s *Service) RunNow(scheduleID int64) (int64, error) {
	if s.Runner == nil {
		return 0, &Error{Status: 409, Msg: "Marchive is not running. Start it with `marchive`."}
	}
	id, err := s.Runner.RunNow(scheduleID)
	if errors.Is(err, store.ErrNotFound) {
		return 0, err
	}
	if err != nil {
		return 0, &Error{Status: 409, Msg: err.Error()}
	}
	return id, nil
}

func (s *Service) SetRunner(on bool) error {
	if s.Runner == nil {
		if err := s.Store.SetRunnerEnabled(on); err != nil {
			return err
		}
		if on {
			return &Error{Status: 409, Msg: "The runner flag is saved, but Marchive is not running. Start it with `marchive`."}
		}
		return nil
	}
	return s.Runner.SetEnabled(on)
}

func (s *Service) CancelRun(id int64) error {
	if s.Runner == nil {
		return &Error{Status: 409, Msg: "Marchive is not running, so there is nothing to cancel."}
	}
	return s.Runner.Cancel(id)
}

func (s *Service) UpdateSettings(in SettingsInput) error {
	if in.PodcastConcurrency != nil {
		if err := s.Store.SetPodcastConcurrency(*in.PodcastConcurrency); err != nil {
			return err
		}
	}
	if in.ListenPort != nil {
		n := *in.ListenPort
		if n < 0 || n > 65535 {
			return &Error{Status: 400, Msg: "choose a port from 1 to 65535, or random"}
		}
		if err := s.Store.SetListenPort(n); err != nil {
			return err
		}
	}
	if in.ItemTimeoutSeconds != nil {
		n := *in.ItemTimeoutSeconds
		if n < 0 || n > store.MaxTimeoutSeconds {
			return &Error{Status: 400, Msg: "Choose an item time limit from 0 to 86400 seconds. 0 means no time limit."}
		}
		if err := s.Store.SetItemTimeoutSeconds(n); err != nil {
			return err
		}
	}
	if in.CaptureTimeoutSeconds != nil {
		n := *in.CaptureTimeoutSeconds
		if n < 0 || n > store.MaxTimeoutSeconds {
			return &Error{Status: 400, Msg: "Choose a capture time limit from 0 to 86400 seconds. 0 means no time limit."}
		}
		if err := s.Store.SetCaptureTimeoutSeconds(n); err != nil {
			return err
		}
	}
	if in.ItemTries != nil {
		n := *in.ItemTries
		if n < 1 || n > store.MaxAttempts {
			return &Error{Status: 400, Msg: "Choose item attempts from 1 to 10. 1 means a single attempt."}
		}
		if err := s.Store.SetItemTries(n); err != nil {
			return err
		}
	}
	if in.CaptureTries != nil {
		n := *in.CaptureTries
		if n < 1 || n > store.MaxAttempts {
			return &Error{Status: 400, Msg: "Choose capture attempts from 1 to 10. 1 means a single attempt."}
		}
		if err := s.Store.SetCaptureTries(n); err != nil {
			return err
		}
	}
	if err := s.Store.SetBrowserConcurrency(1); err != nil {
		return err
	}
	if in.OpenOnLogin != nil || in.RestartOnCrash != nil {
		if err := s.applyLoginService(in); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) applyLoginService(in SettingsInput) error {
	open, restart, err := supervise.Current()
	if err != nil {
		return err
	}
	if in.OpenOnLogin != nil {
		open = *in.OpenOnLogin
	}
	if in.RestartOnCrash != nil {
		restart = *in.RestartOnCrash
	}
	dir := s.DataDir
	if dir == "" {
		dir, err = paths.Resolve("")
		if err != nil {
			return err
		}
	}
	err = supervise.Apply(supervise.Spec{
		DataDir:        dir,
		OpenOnLogin:    open,
		RestartOnCrash: restart,
	})
	if err != nil {
		return &Error{Status: 400, Msg: err.Error()}
	}
	return nil
}

func (s *Service) BeginSignIn() error {
	return s.openChromeWindow(func(br *browser.Client) error { return br.BeginSignIn() })
}

func (s *Service) OpenChrome() error {
	return s.openChromeWindow(func(br *browser.Client) error { return br.OpenChrome() })
}

func (s *Service) openChromeWindow(open func(*browser.Client) error) error {
	if s.Browser == nil {
		return &Error{Status: 409, Msg: "Marchive is not running."}
	}
	if err := open(s.Browser); err != nil {
		return &Error{Status: 409, Msg: err.Error()}
	}
	return nil
}

func (s *Service) ChromeInstall() browser.InstallSnapshot {
	if s.Browser == nil {
		return browser.InstallSnapshot{Phase: "idle", Message: "Marchive is not running."}
	}
	return s.Browser.InstallSnapshot()
}

func (s *Service) StartChromeInstall() error {
	if s.Browser == nil {
		return &Error{Status: 409, Msg: "Marchive is not running."}
	}
	if err := s.Browser.StartInstall(); err != nil {
		return &Error{Status: 409, Msg: err.Error()}
	}
	return nil
}

func (s *Service) CancelChromeInstall() {
	if s.Browser != nil {
		s.Browser.CancelInstall()
	}
}

func (s *Service) YtDlpInstall() ytdlp.Snapshot {
	if s == nil || s.ytdlp == nil {
		return ytdlp.Snapshot{Phase: "idle", Message: "Marchive is not running."}
	}
	return s.ytdlp.Snapshot(s.DataDir)
}

func (s *Service) StartYtDlpInstall(update bool) error {
	if s == nil || s.ytdlp == nil {
		return &Error{Status: 409, Msg: "Marchive is not running."}
	}
	if err := s.ytdlp.Start(s.DataDir, update); err != nil {
		return &Error{Status: 409, Msg: err.Error()}
	}
	return nil
}

func (s *Service) CancelYtDlpInstall() {
	if s != nil && s.ytdlp != nil {
		s.ytdlp.Cancel()
	}
}

func (s *Service) OpenPath(target string) error {
	abs, err := s.archivePath(target)
	if err != nil {
		return err
	}
	return openFolder(abs)
}

// FinderProxy is a small file a browser can drop outside its window.
// Opening it reveals the archive folder in Finder.
func (s *Service) FinderProxy(target string) ([]byte, error) {
	abs, err := s.archivePath(target)
	if err != nil {
		return nil, err
	}
	return finderProxy(abs), nil
}

func (s *Service) archivePath(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", &Error{Status: 400, Msg: "missing path"}
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", &Error{Status: 400, Msg: err.Error()}
	}
	roots, err := s.Store.DownloadRoots()
	if err != nil {
		return "", err
	}
	if !insideAny(roots, abs) {
		return "", &Error{Status: 400, Msg: "that folder is not one of your archive folders"}
	}
	if _, err := os.Stat(abs); err != nil {
		return "", &Error{Status: 400, Msg: "that folder is not on disk yet"}
	}
	return abs, nil
}

func (s *Service) kick() {
	if s.Runner != nil {
		s.Runner.Kick()
	}
}

func cleanDest(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &Error{Status: 400, Msg: "choose a folder for the archive"}
	}
	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		raw = filepath.Join(home, raw[2:])
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	if abs == "/" || (home != "" && abs == home) {
		return "", &Error{Status: 400, Msg: "pick a folder inside your home directory, not the home directory itself"}
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		return "", &Error{Status: 400, Msg: "the archive path is a file, not a folder"}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create archive folder: %w", err)
	}
	return abs, nil
}

func insideAny(roots []string, target string) bool {
	target = filepath.Clean(target)
	for _, root := range roots {
		root = filepath.Clean(root)
		rel, err := filepath.Rel(root, target)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

func openFolder(path string) error {
	return openPath(path)
}
