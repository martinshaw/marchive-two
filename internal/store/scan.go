package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

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

// SetItemsRead records or clears the time each item was read.
func (s *Store) SetItemsRead(ids []int64, read bool) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	now := any(nil)
	if read {
		now = fmtTime(time.Now().UTC())
		args = append(args, now)
	}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := `UPDATE items SET read_at = NULL WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	if read {
		q = `UPDATE items SET read_at = ? WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	}
	_, err := s.db.Exec(q, args...)
	return err
}

func scanItem(sc scanner) (Item, error) {
	var item Item
	var runID sql.NullInt64
	var created, updated string
	var readAt sql.NullString
	err := sc.Scan(
		&item.ID, &item.SourceID, &runID, &item.IdentityKey, &item.Kind, &item.Title, &item.URL,
		&item.Status, &item.Error, &item.DownloadLocation, &item.ContentHash, &item.RetryCount,
		&created, &updated, &readAt,
	)
	if err != nil {
		return Item{}, err
	}
	if runID.Valid {
		item.RunID = runID.Int64
	}
	item.CreatedAt = mustTime(created)
	item.UpdatedAt = mustTime(updated)
	if when := parseTime(readAt); when != nil {
		item.ReadAt = *when
		item.Read = true
	}
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
