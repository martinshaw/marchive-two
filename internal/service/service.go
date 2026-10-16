// Package service is the API the UI and the command line share.
package service

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/runner"
	"github.com/martinshaw/marchive-two/internal/store"
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
}

// SettingsInput changes concurrency. Browser captures stay at one.
type SettingsInput struct {
	PodcastConcurrency *int `json:"podcastConcurrency"`
	BrowserConcurrency *int `json:"browserConcurrency"`
}

// State is the snapshot rendered by the UI.
type State struct {
	RunnerEnabled      bool          `json:"runnerEnabled"`
	SigningIn          bool          `json:"signingIn"`
	BrowserBusy        bool          `json:"browserBusy"`
	ChromePath         string        `json:"chromePath"`
	ChromeError        string        `json:"chromeError"`
	PodcastConcurrency int           `json:"podcastConcurrency"`
	BrowserConcurrency int           `json:"browserConcurrency"`
	Sources            []SourceState `json:"sources"`
	Runs               []RunState    `json:"runs"`
	Error              string        `json:"error,omitempty"`
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
}

func New(st *store.Store, run *runner.Runner, br *browser.Client) *Service {
	return &Service{Store: st, Runner: run, Browser: br}
}

func (s *Service) State() (State, error) {
	st := State{BrowserConcurrency: 1, PodcastConcurrency: 2}
	var err error
	st.RunnerEnabled, err = s.Store.RunnerEnabled()
	if err != nil {
		return State{}, err
	}
	st.PodcastConcurrency, err = s.Store.PodcastConcurrency()
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
		return 0, 0, &Error{Status: 400, Msg: "choose a webpage, article index, podcast, or text extract"}
	}
	if err := provider.ValidateSelector(in.LinkSelector); err != nil {
		return 0, 0, &Error{Status: 400, Msg: "link selector is not valid CSS"}
	}
	if in.Provider != provider.KindArticles {
		in.LinkSelector = ""
	}
	if in.IntervalSeconds < 30 {
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

func (s *Service) DeleteSource(id int64) error {
	if err := s.Store.DeleteSource(id); err != nil {
		return err
	}
	s.kick()
	return nil
}

func (s *Service) UpdateSchedule(id int64, in ScheduleInput) error {
	current, err := s.Store.GetSchedule(id)
	if err != nil {
		return err
	}
	if in.IntervalSeconds != nil {
		if *in.IntervalSeconds < 30 {
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
	if err := s.Store.UpdateSchedule(current.Schedule); err != nil {
		return err
	}
	s.kick()
	return nil
}

func (s *Service) DeleteSchedule(id int64) error {
	if err := s.Store.DeleteSchedule(id); err != nil {
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
	if err := s.Store.SetBrowserConcurrency(1); err != nil {
		return err
	}
	return nil
}

func (s *Service) BeginSignIn() error {
	if s.Browser == nil {
		return &Error{Status: 409, Msg: "Marchive is not running."}
	}
	if err := s.Browser.BeginSignIn(); err != nil {
		return &Error{Status: 409, Msg: err.Error()}
	}
	return nil
}

func (s *Service) OpenPath(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return &Error{Status: 400, Msg: "missing path"}
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return &Error{Status: 400, Msg: err.Error()}
	}
	roots, err := s.Store.DownloadRoots()
	if err != nil {
		return err
	}
	if !insideAny(roots, abs) {
		return &Error{Status: 400, Msg: "that folder is not one of your archive folders"}
	}
	if _, err := os.Stat(abs); err != nil {
		return &Error{Status: 400, Msg: "that folder is not on disk yet"}
	}
	return openFolder(abs)
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
