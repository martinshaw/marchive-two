// Package store is the SQLite record of sources, schedules, runs, and items.
package store

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusSkipped   = "skipped"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

// Source is a URL captured by one provider.
type Source struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	Provider     string    `json:"provider"`
	LinkSelector string    `json:"linkSelector"`
	Capture      string    `json:"capture"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Schedule says how often a source is captured and where files go.
type Schedule struct {
	ID               int64      `json:"id"`
	SourceID         int64      `json:"sourceId"`
	IntervalSeconds  int        `json:"intervalSeconds"`
	DownloadLocation string     `json:"downloadLocation"`
	Enabled          bool       `json:"enabled"`
	MaxNew           int        `json:"maxNew"`
	LastRunAt        *time.Time `json:"lastRunAt"`
	NextRunAt        *time.Time `json:"nextRunAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// EveryCount is the interval amount in EveryUnit.
func (s Schedule) EveryCount() int {
	if s.IntervalSeconds%3600 == 0 && s.IntervalSeconds >= 3600 {
		return s.IntervalSeconds / 3600
	}
	if s.IntervalSeconds%60 == 0 && s.IntervalSeconds >= 60 {
		return s.IntervalSeconds / 60
	}
	if s.IntervalSeconds == 0 {
		return 1
	}
	return s.IntervalSeconds
}

// EveryUnit is hours, minutes, seconds, or once.
func (s Schedule) EveryUnit() string {
	if s.IntervalSeconds == 0 {
		return "once"
	}
	if s.IntervalSeconds%3600 == 0 && s.IntervalSeconds >= 3600 {
		return "hours"
	}
	if s.IntervalSeconds%60 == 0 && s.IntervalSeconds >= 60 {
		return "minutes"
	}
	return "seconds"
}

// Run is one execution of a schedule.
type Run struct {
	ID         int64      `json:"id"`
	ScheduleID int64      `json:"scheduleId"`
	Status     string     `json:"status"`
	Error      string     `json:"error"`
	Note       string     `json:"note"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// RunDetail joins a run to the source it captured.
type RunDetail struct {
	Run
	SourceID   int64  `json:"sourceId"`
	SourceName string `json:"sourceName"`
	SourceURL  string `json:"sourceUrl"`
	Provider   string `json:"provider"`
}

// ScheduleDetail joins a schedule to its source.
type ScheduleDetail struct {
	Schedule
	Source Source `json:"source"`
}

// Item is one archived article, episode, or snapshot.
type Item struct {
	ID               int64     `json:"id"`
	SourceID         int64     `json:"sourceId"`
	RunID            int64     `json:"runId"`
	IdentityKey      string    `json:"identityKey"`
	Kind             string    `json:"kind"`
	Title            string    `json:"title"`
	URL              string    `json:"url"`
	Status           string    `json:"status"`
	Error            string    `json:"error"`
	DownloadLocation string    `json:"downloadLocation"`
	ContentHash      string    `json:"contentHash"`
	RetryCount       int       `json:"retryCount"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	ReadAt           time.Time `json:"-"`
	Read             bool      `json:"read"`
}

// Store is a single SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens or creates the database and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func dsn(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-20000)&_pragma=temp_store(MEMORY)"
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
