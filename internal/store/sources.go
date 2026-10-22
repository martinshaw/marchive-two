package store

import (
	"database/sql"
	"errors"
	"time"
)

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

func (s *Store) SetSourceLinkSelector(id int64, selector string) error {
	res, err := s.db.Exec(`UPDATE sources SET link_selector = ?, updated_at = ? WHERE id = ?`, selector, fmtTime(time.Now().UTC()), id)
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
