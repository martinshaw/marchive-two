// Package store is the SQLite record of sources, schedules, runs, and items.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// EveryUnit is hours, minutes, or seconds.
func (s Schedule) EveryUnit() string {
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

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	return s.ensureColumn("sources", "capture", "TEXT NOT NULL DEFAULT ''")
}

func (s *Store) ensureColumn(table, column, decl string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + decl)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS sources (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  url TEXT NOT NULL,
  provider TEXT NOT NULL,
  link_selector TEXT NOT NULL DEFAULT '',
  capture TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS schedules (
  id INTEGER PRIMARY KEY,
  source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  interval_seconds INTEGER NOT NULL,
  download_location TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  max_new INTEGER NOT NULL DEFAULT 20,
  last_run_at TEXT,
  next_run_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY,
  schedule_id INTEGER NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  started_at TEXT,
  finished_at TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS items (
  id INTEGER PRIMARY KEY,
  source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  run_id INTEGER REFERENCES runs(id) ON DELETE SET NULL,
  identity_key TEXT NOT NULL,
  kind TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  url TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  download_location TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  retry_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(source_id, identity_key)
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_schedules_next ON schedules(enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_runs_schedule_status ON runs(schedule_id, status);
CREATE INDEX IF NOT EXISTS idx_items_run ON items(run_id);
CREATE INDEX IF NOT EXISTS idx_items_source_kind ON items(source_id, kind, id);
`

func (s *Store) CreateSourceWithSchedule(src Source, sch Schedule) (int64, int64, error) {
	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO sources (name, url, provider, link_selector, capture, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		src.Name, src.URL, src.Provider, src.LinkSelector, src.Capture, fmtTime(now), fmtTime(now))
	if err != nil {
		return 0, 0, err
	}
	sourceID, err := res.LastInsertId()
	if err != nil {
		return 0, 0, err
	}
	next := now
	res, err = tx.Exec(`
		INSERT INTO schedules (
			source_id, interval_seconds, download_location, enabled, max_new,
			next_run_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sourceID, sch.IntervalSeconds, sch.DownloadLocation, boolInt(sch.Enabled), sch.MaxNew,
		fmtTime(next), fmtTime(now), fmtTime(now))
	if err != nil {
		return 0, 0, err
	}
	scheduleID, err := res.LastInsertId()
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return sourceID, scheduleID, nil
}

func (s *Store) ListSources() ([]Source, error) {
	rows, err := s.db.Query(`SELECT id, name, url, provider, link_selector, capture, created_at, updated_at FROM sources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) GetSource(id int64) (Source, error) {
	row := s.db.QueryRow(`SELECT id, name, url, provider, link_selector, capture, created_at, updated_at FROM sources WHERE id = ?`, id)
	src, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, ErrNotFound
	}
	return src, err
}

func (s *Store) DeleteSource(id int64) error {
	res, err := s.db.Exec(`DELETE FROM sources WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetSourceName(id int64, name string) error {
	_, err := s.db.Exec(`UPDATE sources SET name = ?, updated_at = ? WHERE id = ?`, name, fmtTime(time.Now().UTC()), id)
	return err
}

func (s *Store) SetSourceCapture(id int64, capture string) error {
	res, err := s.db.Exec(`UPDATE sources SET capture = ?, updated_at = ? WHERE id = ?`, capture, fmtTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListSchedules() ([]ScheduleDetail, error) {
	rows, err := s.db.Query(scheduleSelect + ` ORDER BY schedules.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduleDetails(rows)
}

func (s *Store) GetSchedule(id int64) (ScheduleDetail, error) {
	row := s.db.QueryRow(scheduleSelect+` WHERE schedules.id = ?`, id)
	d, err := scanScheduleDetail(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduleDetail{}, ErrNotFound
	}
	return d, err
}

func (s *Store) UpdateSchedule(sch Schedule) error {
	res, err := s.db.Exec(`
		UPDATE schedules
		SET interval_seconds = ?, download_location = ?, enabled = ?, max_new = ?, updated_at = ?
		WHERE id = ?`,
		sch.IntervalSeconds, sch.DownloadLocation, boolInt(sch.Enabled), sch.MaxNew, fmtTime(time.Now().UTC()), sch.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteSchedule(id int64) error {
	res, err := s.db.Exec(`DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DueSchedules(now time.Time) ([]ScheduleDetail, error) {
	rows, err := s.db.Query(scheduleSelect+`
		WHERE schedules.enabled = 1
		  AND schedules.next_run_at IS NOT NULL
		  AND schedules.next_run_at <= ?
		  AND NOT EXISTS (
		    SELECT 1 FROM runs
		    WHERE runs.schedule_id = schedules.id
		      AND runs.status IN (?, ?)
		  )
		ORDER BY schedules.next_run_at`, fmtTime(now.UTC()), StatusQueued, StatusRunning)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduleDetails(rows)
}

func (s *Store) NextWake(now time.Time) (time.Duration, bool) {
	row := s.db.QueryRow(`
		SELECT schedules.next_run_at
		FROM schedules
		WHERE schedules.enabled = 1
		  AND schedules.next_run_at IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM runs
		    WHERE runs.schedule_id = schedules.id
		      AND runs.status IN (?, ?)
		  )
		ORDER BY schedules.next_run_at
		LIMIT 1`, StatusQueued, StatusRunning)
	var raw sql.NullString
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false
		}
		return time.Minute, true
	}
	next := parseTime(raw)
	if next == nil {
		return 0, false
	}
	d := next.Sub(now)
	if d < 0 {
		return 0, true
	}
	return d, true
}

func (s *Store) AdvanceSchedule(id int64, now time.Time) error {
	var interval int
	err := s.db.QueryRow(`SELECT interval_seconds FROM schedules WHERE id = ?`, id).Scan(&interval)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if interval < 1 {
		interval = 1
	}
	next := now.UTC().Add(time.Duration(interval) * time.Second)
	_, err = s.db.Exec(`
		UPDATE schedules SET last_run_at = ?, next_run_at = ?, updated_at = ? WHERE id = ?`,
		fmtTime(now.UTC()), fmtTime(next), fmtTime(now.UTC()), id)
	return err
}

func (s *Store) HasActiveRun(scheduleID int64) (bool, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(1) FROM runs WHERE schedule_id = ? AND status IN (?, ?)`,
		scheduleID, StatusQueued, StatusRunning).Scan(&n)
	return n > 0, err
}

func (s *Store) CreateRun(scheduleID int64) (Run, error) {
	now := time.Now().UTC()
	res, err := s.db.Exec(`
		INSERT INTO runs (schedule_id, status, error, note, created_at) VALUES (?, ?, '', '', ?)`,
		scheduleID, StatusQueued, fmtTime(now))
	if err != nil {
		return Run{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Run{}, err
	}
	return s.GetRun(id)
}

func (s *Store) GetRun(id int64) (Run, error) {
	row := s.db.QueryRow(`
		SELECT id, schedule_id, status, error, note, started_at, finished_at, created_at
		FROM runs WHERE id = ?`, id)
	run, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return run, err
}

func (s *Store) ListRunDetails(limit int) ([]RunDetail, error) {
	if limit < 1 {
		limit = 40
	}
	rows, err := s.db.Query(`
		SELECT runs.id, runs.schedule_id, runs.status, runs.error, runs.note,
		       runs.started_at, runs.finished_at, runs.created_at,
		       sources.id, sources.name, sources.url, sources.provider
		FROM runs
		JOIN schedules ON schedules.id = runs.schedule_id
		JOIN sources ON sources.id = schedules.source_id
		ORDER BY runs.id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunDetail
	for rows.Next() {
		d, err := scanRunDetail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ClaimRun(id int64) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE runs SET status = ?, started_at = ? WHERE id = ? AND status = ?`,
		StatusRunning, fmtTime(time.Now().UTC()), id, StatusQueued)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *Store) FinishRun(id int64, status, errMsg, note string) (bool, error) {
	return s.CompleteRun(id, 0, status, errMsg, note, time.Time{})
}

// CompleteRun marks a queued or running capture finished and, when it changed,
// schedules the next run.
func (s *Store) CompleteRun(runID, scheduleID int64, status, errMsg, note string, now time.Time) (bool, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		UPDATE runs SET status = ?, error = ?, note = ?, finished_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		status, errMsg, note, fmtTime(now), runID, StatusQueued, StatusRunning)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 1 && scheduleID != 0 {
		var interval int
		err = tx.QueryRow(`SELECT interval_seconds FROM schedules WHERE id = ?`, scheduleID).Scan(&interval)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if err == nil {
			if interval < 1 {
				interval = 1
			}
			next := now.Add(time.Duration(interval) * time.Second)
			if _, err := tx.Exec(`
				UPDATE schedules SET last_run_at = ?, next_run_at = ?, updated_at = ? WHERE id = ?`,
				fmtTime(now), fmtTime(next), fmtTime(now), scheduleID); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n == 1, nil
}

func (s *Store) ListRunIDsByStatus(status string) ([]int64, error) {
	rows, err := s.db.Query(`SELECT id FROM runs WHERE status = ? ORDER BY id`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) FailInterrupted() error {
	now := fmtTime(time.Now().UTC())
	msg := "Interrupted because Marchive stopped."
	if _, err := s.db.Exec(`
		UPDATE runs SET status = ?, error = ?, finished_at = ?
		WHERE status IN (?, ?)`, StatusFailed, msg, now, StatusQueued, StatusRunning); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		UPDATE items SET status = ?, error = ?, updated_at = ?
		WHERE status IN (?, ?)`, StatusFailed, msg, now, StatusQueued, StatusRunning)
	return err
}

func (s *Store) InsertItem(item Item) (int64, error) {
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	var runID any
	if item.RunID > 0 {
		runID = item.RunID
	}
	res, err := s.db.Exec(`
		INSERT INTO items (
			source_id, run_id, identity_key, kind, title, url, status, error,
			download_location, content_hash, retry_count, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.SourceID, runID, item.IdentityKey, item.Kind, item.Title, item.URL, item.Status, item.Error,
		item.DownloadLocation, item.ContentHash, item.RetryCount, fmtTime(item.CreatedAt), fmtTime(item.UpdatedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrExists
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateItem(item Item) error {
	_, err := s.db.Exec(`
		UPDATE items
		SET status = ?, error = ?, download_location = ?, content_hash = ?, retry_count = ?, title = ?, url = ?, updated_at = ?
		WHERE id = ?`,
		item.Status, item.Error, item.DownloadLocation, item.ContentHash, item.RetryCount, item.Title, item.URL,
		fmtTime(time.Now().UTC()), item.ID)
	return err
}

func (s *Store) ItemSeen(sourceID int64, keys ...string) (bool, error) {
	var filtered []string
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			filtered = append(filtered, key)
		}
	}
	if len(filtered) == 0 {
		return false, nil
	}
	placeholders := make([]string, len(filtered))
	args := make([]any, 0, 1+len(filtered)*2)
	args = append(args, sourceID)
	for i, key := range filtered {
		placeholders[i] = "?"
		args = append(args, key)
	}
	ident := strings.Join(placeholders, ",")
	q := `SELECT 1 FROM items WHERE source_id = ? AND (identity_key IN (` + ident + `) OR url IN (` + ident + `)) LIMIT 1`
	args = append(args, anySlice(filtered)...)
	var one int
	err := s.db.QueryRow(q, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) LatestContentHash(sourceID int64, kind string) (string, bool, error) {
	var hash string
	err := s.db.QueryRow(`
		SELECT content_hash FROM items
		WHERE source_id = ? AND kind = ? AND content_hash != ''
		ORDER BY id DESC LIMIT 1`, sourceID, kind).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}

func (s *Store) ListItemsByRunIDs(ids []int64) ([]Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.Query(`
		SELECT id, source_id, run_id, identity_key, kind, title, url, status, error,
		       download_location, content_hash, retry_count, created_at, updated_at
		FROM items WHERE run_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CancelOpenItems(runID int64) error {
	_, err := s.db.Exec(`
		UPDATE items SET status = ?, updated_at = ? WHERE run_id = ? AND status IN (?, ?)`,
		StatusCancelled, fmtTime(time.Now().UTC()), runID, StatusQueued, StatusRunning)
	return err
}

func (s *Store) FailOpenItems(runID int64, msg string) error {
	_, err := s.db.Exec(`
		UPDATE items SET status = ?, error = ?, updated_at = ? WHERE run_id = ? AND status IN (?, ?)`,
		StatusFailed, msg, fmtTime(time.Now().UTC()), runID, StatusQueued, StatusRunning)
	return err
}

func (s *Store) DownloadRoots() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT download_location FROM schedules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			return nil, err
		}
		out = append(out, root)
	}
	return out, rows.Err()
}

func (s *Store) GetSetting(key, fallback string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	return value, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) RunnerEnabled() (bool, error) {
	v, err := s.GetSetting("runner_enabled", "false")
	return v == "true", err
}

func (s *Store) SetRunnerEnabled(on bool) error {
	return s.SetSetting("runner_enabled", boolString(on))
}

func (s *Store) PodcastConcurrency() (int, error) {
	v, err := s.GetSetting("podcast_concurrency", "2")
	if err != nil {
		return 2, err
	}
	n, err := parsePositive(v, 2)
	if n < 1 {
		n = 1
	}
	if n > 6 {
		n = 6
	}
	return n, err
}

func (s *Store) SetPodcastConcurrency(n int) error {
	if n < 1 {
		n = 1
	}
	if n > 6 {
		n = 6
	}
	return s.SetSetting("podcast_concurrency", fmt.Sprintf("%d", n))
}

// ListenPort is the UI port to bind. Zero means the operating system picks one.
func (s *Store) ListenPort() (int, error) {
	v, err := s.GetSetting("listen_port", "0")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 || n > 65535 {
		return 0, nil
	}
	return n, nil
}

// SetListenPort stores the UI port. Zero asks for a random port on the next start.
func (s *Store) SetListenPort(n int) error {
	if n < 0 || n > 65535 {
		return fmt.Errorf("port %d is outside 0 to 65535", n)
	}
	return s.SetSetting("listen_port", strconv.Itoa(n))
}

func (s *Store) BrowserConcurrency() (int, error) {
	_, err := s.GetSetting("browser_concurrency", "1")
	return 1, err
}

func (s *Store) SetBrowserConcurrency(n int) error {
	return s.SetSetting("browser_concurrency", "1")
}

const scheduleSelect = `
SELECT schedules.id, schedules.source_id, schedules.interval_seconds, schedules.download_location,
       schedules.enabled, schedules.max_new, schedules.last_run_at, schedules.next_run_at,
       schedules.created_at, schedules.updated_at,
       sources.id, sources.name, sources.url, sources.provider, sources.link_selector, sources.capture,
       sources.created_at, sources.updated_at
FROM schedules
JOIN sources ON sources.id = schedules.source_id
`

type scanner interface {
	Scan(dest ...any) error
}

func scanSource(sc scanner) (Source, error) {
	var src Source
	var created, updated string
	err := sc.Scan(&src.ID, &src.Name, &src.URL, &src.Provider, &src.LinkSelector, &src.Capture, &created, &updated)
	if err != nil {
		return Source{}, err
	}
	src.CreatedAt = mustTime(created)
	src.UpdatedAt = mustTime(updated)
	return src, nil
}

func scanScheduleDetail(sc scanner) (ScheduleDetail, error) {
	var d ScheduleDetail
	var enabled int
	var last, next sql.NullString
	var schCreated, schUpdated, srcCreated, srcUpdated string
	err := sc.Scan(
		&d.ID, &d.SourceID, &d.IntervalSeconds, &d.DownloadLocation, &enabled, &d.MaxNew,
		&last, &next, &schCreated, &schUpdated,
		&d.Source.ID, &d.Source.Name, &d.Source.URL, &d.Source.Provider, &d.Source.LinkSelector, &d.Source.Capture,
		&srcCreated, &srcUpdated,
	)
	if err != nil {
		return ScheduleDetail{}, err
	}
	d.Enabled = enabled == 1
	d.LastRunAt = parseTime(last)
	d.NextRunAt = parseTime(next)
	d.CreatedAt = mustTime(schCreated)
	d.UpdatedAt = mustTime(schUpdated)
	d.Source.CreatedAt = mustTime(srcCreated)
	d.Source.UpdatedAt = mustTime(srcUpdated)
	return d, nil
}

func scanScheduleDetails(rows *sql.Rows) ([]ScheduleDetail, error) {
	var out []ScheduleDetail
	for rows.Next() {
		d, err := scanScheduleDetail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func scanRun(sc scanner) (Run, error) {
	var run Run
	var started, finished sql.NullString
	var created string
	err := sc.Scan(&run.ID, &run.ScheduleID, &run.Status, &run.Error, &run.Note, &started, &finished, &created)
	if err != nil {
		return Run{}, err
	}
	run.StartedAt = parseTime(started)
	run.FinishedAt = parseTime(finished)
	run.CreatedAt = mustTime(created)
	return run, nil
}

func scanRunDetail(sc scanner) (RunDetail, error) {
	var d RunDetail
	var started, finished sql.NullString
	var created string
	err := sc.Scan(
		&d.ID, &d.ScheduleID, &d.Status, &d.Error, &d.Note, &started, &finished, &created,
		&d.SourceID, &d.SourceName, &d.SourceURL, &d.Provider,
	)
	if err != nil {
		return RunDetail{}, err
	}
	d.StartedAt = parseTime(started)
	d.FinishedAt = parseTime(finished)
	d.CreatedAt = mustTime(created)
	return d, nil
}

func scanItem(sc scanner) (Item, error) {
	var item Item
	var runID sql.NullInt64
	var created, updated string
	err := sc.Scan(
		&item.ID, &item.SourceID, &runID, &item.IdentityKey, &item.Kind, &item.Title, &item.URL,
		&item.Status, &item.Error, &item.DownloadLocation, &item.ContentHash, &item.RetryCount,
		&created, &updated,
	)
	if err != nil {
		return Item{}, err
	}
	if runID.Valid {
		item.RunID = runID.Int64
	}
	item.CreatedAt = mustTime(created)
	item.UpdatedAt = mustTime(updated)
	return item, nil
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, v.String)
	if err != nil {
		return nil
	}
	return &t
}

func mustTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func parsePositive(v string, fallback int) (int, error) {
	var n int
	_, err := fmt.Sscan(v, &n)
	if err != nil {
		return fallback, nil
	}
	return n, nil
}

func anySlice(v []string) []any {
	out := make([]any, len(v))
	for i := range v {
		out[i] = v[i]
	}
	return out
}
