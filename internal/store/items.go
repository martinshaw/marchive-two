package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

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

// ItemDir is the folder of an item already saved for this source and address.
func (s *Store) ItemDir(sourceID int64, key string) (string, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, nil
	}
	var dir string
	err := s.db.QueryRow(`
		SELECT download_location FROM items
		WHERE source_id = ? AND download_location != '' AND (identity_key = ? OR url = ?)
		ORDER BY id LIMIT 1`, sourceID, key, key).Scan(&dir)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return dir, true, nil
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

func (s *Store) GetItem(id int64) (Item, error) {
	row := s.db.QueryRow(`
		SELECT id, source_id, run_id, identity_key, kind, title, url, status, error,
		       download_location, content_hash, retry_count, created_at, updated_at, read_at
		FROM items WHERE id = ?`, id)
	item, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return item, err
}

// LocatedItem is a captured item and the source name used to group its files.
type LocatedItem struct {
	ID               int64
	SourceID         int64
	SourceName       string
	DownloadLocation string
}

func (s *Store) ListLocatedItems() ([]LocatedItem, error) {
	rows, err := s.db.Query(`
		SELECT items.id, items.source_id, sources.name, items.download_location
		FROM items
		JOIN sources ON sources.id = items.source_id
		WHERE items.download_location != ''
		ORDER BY items.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LocatedItem
	for rows.Next() {
		var item LocatedItem
		if err := rows.Scan(&item.ID, &item.SourceID, &item.SourceName, &item.DownloadLocation); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SourceArchiveRoots() (map[int64][]string, error) {
	rows, err := s.db.Query(`SELECT source_id, download_location FROM schedules WHERE download_location != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]string{}
	for rows.Next() {
		var id int64
		var dir string
		if err := rows.Scan(&id, &dir); err != nil {
			return nil, err
		}
		out[id] = append(out[id], dir)
	}
	return out, rows.Err()
}

func (s *Store) SetItemDownloadLocation(id int64, dir string) error {
	res, err := s.db.Exec(`UPDATE items SET download_location = ?, updated_at = ? WHERE id = ?`, dir, fmtTime(time.Now().UTC()), id)
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

// WordItem is an archived item that may have a words.json file, with the capture it belongs to.
type WordItem struct {
	ID               int64
	RunID            int64
	Title            string
	URL              string
	DownloadLocation string
	RunAt            time.Time
	SourceName       string
	SourceURL        string
	Provider         string
}

func (s *Store) ListWordItems() ([]WordItem, error) {
	rows, err := s.db.Query(`
		SELECT items.id, items.run_id, items.title, items.url, items.download_location,
		       runs.created_at, sources.name, sources.url, sources.provider
		FROM items
		JOIN runs ON runs.id = items.run_id
		JOIN schedules ON schedules.id = runs.schedule_id
		JOIN sources ON sources.id = schedules.source_id
		WHERE items.download_location != ''
		ORDER BY runs.created_at, runs.id, items.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WordItem
	for rows.Next() {
		var item WordItem
		var created string
		if err := rows.Scan(&item.ID, &item.RunID, &item.Title, &item.URL, &item.DownloadLocation, &created, &item.SourceName, &item.SourceURL, &item.Provider); err != nil {
			return nil, err
		}
		item.RunAt = mustTime(created)
		out = append(out, item)
	}
	return out, rows.Err()
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
		       download_location, content_hash, retry_count, created_at, updated_at, read_at
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
