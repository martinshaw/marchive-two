package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

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

// MaxTimeoutSeconds is the longest time limit a setting may store.
// Zero is stored separately and means no time limit.
const MaxTimeoutSeconds = 24 * 60 * 60

// MaxAttempts is the most times a request or a capture may be tried.
const MaxAttempts = 10

// ItemTimeoutSeconds is how long one page, file, or request may take.
// Zero means no time limit.
func (s *Store) ItemTimeoutSeconds() (int, error) {
	return s.timeoutSeconds("item_timeout_seconds")
}

// SetItemTimeoutSeconds stores the per-request time limit. Zero means no time limit.
func (s *Store) SetItemTimeoutSeconds(n int) error {
	if n < 0 || n > MaxTimeoutSeconds {
		return fmt.Errorf("item time limit %d is outside 0 to %d seconds", n, MaxTimeoutSeconds)
	}
	return s.SetSetting("item_timeout_seconds", strconv.Itoa(n))
}

// CaptureTimeoutSeconds is how long one capture may take.
// Zero means no time limit.
func (s *Store) CaptureTimeoutSeconds() (int, error) {
	return s.timeoutSeconds("capture_timeout_seconds")
}

// SetCaptureTimeoutSeconds stores the per-capture time limit. Zero means no time limit.
func (s *Store) SetCaptureTimeoutSeconds(n int) error {
	if n < 0 || n > MaxTimeoutSeconds {
		return fmt.Errorf("capture time limit %d is outside 0 to %d seconds", n, MaxTimeoutSeconds)
	}
	return s.SetSetting("capture_timeout_seconds", strconv.Itoa(n))
}

// ItemTries is how many times to try one page, file, or request.
func (s *Store) ItemTries() (int, error) {
	return s.attemptCount("item_tries")
}

// SetItemTries stores the per-request attempt count.
func (s *Store) SetItemTries(n int) error {
	if n < 1 || n > MaxAttempts {
		return fmt.Errorf("item attempts %d is outside 1 to %d", n, MaxAttempts)
	}
	return s.SetSetting("item_tries", strconv.Itoa(n))
}

// CaptureTries is how many times to try one capture.
func (s *Store) CaptureTries() (int, error) {
	return s.attemptCount("capture_tries")
}

// SetCaptureTries stores the per-capture attempt count.
func (s *Store) SetCaptureTries(n int) error {
	if n < 1 || n > MaxAttempts {
		return fmt.Errorf("capture attempts %d is outside 1 to %d", n, MaxAttempts)
	}
	return s.SetSetting("capture_tries", strconv.Itoa(n))
}

func (s *Store) timeoutSeconds(key string) (int, error) {
	v, err := s.GetSetting(key, "0")
	if err != nil {
		return 0, err
	}
	n, conv := strconv.Atoi(strings.TrimSpace(v))
	if conv != nil || n < 0 || n > MaxTimeoutSeconds {
		return 0, nil
	}
	return n, nil
}

func (s *Store) attemptCount(key string) (int, error) {
	v, err := s.GetSetting(key, "1")
	if err != nil {
		return 1, err
	}
	n, conv := strconv.Atoi(strings.TrimSpace(v))
	if conv != nil || n < 1 {
		return 1, nil
	}
	if n > MaxAttempts {
		return MaxAttempts, nil
	}
	return n, nil
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
