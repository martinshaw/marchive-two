package store

import (
	"database/sql"
	"errors"
	"time"
)

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

func (s *Store) ActiveRunIDs(sourceID, scheduleID int64) ([]int64, error) {
	q := `SELECT runs.id FROM runs JOIN schedules ON schedules.id = runs.schedule_id WHERE runs.status IN (?, ?)`
	args := []any{StatusQueued, StatusRunning}
	if sourceID > 0 {
		q += ` AND schedules.source_id = ?`
		args = append(args, sourceID)
	}
	if scheduleID > 0 {
		q += ` AND runs.schedule_id = ?`
		args = append(args, scheduleID)
	}
	q += ` ORDER BY runs.id`
	return s.queryIDs(q, args...)
}

func (s *Store) queryIDs(q string, args ...any) ([]int64, error) {
	rows, err := s.db.Query(q, args...)
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

func (s *Store) ItemDirs(clause string, args ...any) ([]string, error) {
	q := `SELECT DISTINCT download_location FROM items WHERE download_location != ''`
	if clause != "" {
		q += ` AND (` + clause + `)`
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dirs []string
	for rows.Next() {
		var dir string
		if err := rows.Scan(&dir); err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	return dirs, rows.Err()
}

// Reset removes every source, capture, item, and setting.
func (s *Store) Reset() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`DELETE FROM items`,
		`DELETE FROM runs`,
		`DELETE FROM schedules`,
		`DELETE FROM sources`,
		`DELETE FROM settings`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteItem(id int64) error {
	res, err := s.db.Exec(`DELETE FROM items WHERE id = ?`, id)
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

func (s *Store) DeleteRun(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM items WHERE run_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM runs WHERE id = ?`, id)
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
	return tx.Commit()
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
	if interval == 0 {
		_, err = s.db.Exec(`
			UPDATE schedules SET last_run_at = ?, next_run_at = NULL, enabled = 0, updated_at = ? WHERE id = ?`,
			fmtTime(now.UTC()), fmtTime(now.UTC()), id)
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
