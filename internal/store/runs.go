package store

import (
	"database/sql"
	"errors"
	"time"
)

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

func (s *Store) GetRunDetail(id int64) (RunDetail, error) {
	row := s.db.QueryRow(`
		SELECT runs.id, runs.schedule_id, runs.status, runs.error, runs.note,
		       runs.started_at, runs.finished_at, runs.created_at,
		       sources.id, sources.name, sources.url, sources.provider
		FROM runs
		JOIN schedules ON schedules.id = runs.schedule_id
		JOIN sources ON sources.id = schedules.source_id
		WHERE runs.id = ?`, id)
	detail, err := scanRunDetail(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RunDetail{}, ErrNotFound
	}
	return detail, err
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
			if interval == 0 {
				if _, err := tx.Exec(`
					UPDATE schedules SET last_run_at = ?, next_run_at = NULL, enabled = 0, updated_at = ? WHERE id = ?`,
					fmtTime(now), fmtTime(now), scheduleID); err != nil {
					return false, err
				}
			} else {
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
