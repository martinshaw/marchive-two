package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinshaw/marchive-two/internal/paths"
	"github.com/martinshaw/marchive-two/internal/store"
	"github.com/martinshaw/marchive-two/internal/supervise"
)

// DeleteSource removes a source. When files is set, each captured item folder
// is removed. The shared archive folder itself is left in place.
func (s *Service) DeleteSource(id int64, files bool) error {
	return s.remove("source", id, files)
}

// DeleteSchedule removes a schedule. When files is set, item folders from its
// captures are removed. The archive folder itself is left in place.
func (s *Service) DeleteSchedule(id int64, files bool) error {
	return s.remove("schedule", id, files)
}

// DeleteRun removes a capture and the items it produced. Files stay unless
// files is set.
func (s *Service) DeleteRun(id int64, files bool) error {
	return s.remove("run", id, files)
}

// DeleteItem removes one captured item. Files stay unless files is set.
func (s *Service) DeleteItem(id int64, files bool) error {
	return s.remove("item", id, files)
}

func (s *Service) remove(kind string, id int64, files bool) error {
	dirs, roots, err := s.removalTargets(kind, id, files)
	if err != nil {
		return err
	}
	s.stopActive(kind, id)
	switch kind {
	case "source":
		err = s.Store.DeleteSource(id)
	case "schedule":
		err = s.Store.DeleteSchedule(id)
	case "run":
		err = s.Store.DeleteRun(id)
	case "item":
		err = s.Store.DeleteItem(id)
	default:
		err = &Error{Status: 400, Msg: "unknown remove target"}
	}
	if err != nil {
		return err
	}
	if files {
		if err := removeItemDirs(dirs, roots); err != nil {
			return err
		}
	}
	s.kick()
	return nil
}

func (s *Service) removalTargets(kind string, id int64, files bool) (dirs, roots []string, err error) {
	if !files {
		return nil, nil, nil
	}
	roots, err = s.Store.DownloadRoots()
	if err != nil {
		return nil, nil, err
	}
	switch kind {
	case "source":
		dirs, err = s.Store.ItemDirs("source_id = ?", id)
	case "schedule":
		dirs, err = s.Store.ItemDirs("run_id IN (SELECT id FROM runs WHERE schedule_id = ?)", id)
	case "run":
		dirs, err = s.Store.ItemDirs("run_id = ?", id)
	case "item":
		item, getErr := s.Store.GetItem(id)
		if getErr != nil {
			return nil, nil, getErr
		}
		if item.DownloadLocation != "" {
			dirs = []string{item.DownloadLocation}
		}
	}
	return dirs, roots, err
}

func (s *Service) stopActive(kind string, id int64) {
	if s.Runner == nil {
		return
	}
	var ids []int64
	var err error
	switch kind {
	case "source":
		ids, err = s.Store.ActiveRunIDs(id, 0)
	case "schedule":
		ids, err = s.Store.ActiveRunIDs(0, id)
	case "run":
		ids = []int64{id}
	}
	if err != nil {
		return
	}
	for _, runID := range ids {
		_ = s.Runner.Cancel(runID)
	}
}

// Reset stops the runner, deletes captured files, and erases app data so the
// library is empty. The database file, lock, and current port stay so this
// process can keep serving.
func (s *Service) Reset() error {
	if s.Runner != nil {
		if err := s.Runner.SetEnabled(false); err != nil {
			return err
		}
		running, err := s.Store.ListRunIDsByStatus(store.StatusRunning)
		if err != nil {
			return err
		}
		for _, id := range running {
			_ = s.Runner.Cancel(id)
		}
	}
	if s.Browser != nil {
		s.Browser.CloseProfile()
	}
	dirs, err := s.Store.ItemDirs("1 = 1")
	if err != nil {
		return err
	}
	roots, err := s.Store.DownloadRoots()
	if err != nil {
		return err
	}
	if err := removeItemDirs(dirs, roots); err != nil {
		return err
	}
	for _, dir := range dirs {
		removeEmptyParents(dir, roots)
	}
	if err := s.Store.Reset(); err != nil {
		return err
	}
	if err := removeAppData(s.DataDir); err != nil {
		return err
	}
	return s.clearLoginService()
}

func removeAppData(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	var first error
	for _, path := range []string{paths.Profile(dir), filepath.Join(dir, "bin"), paths.Log(dir)} {
		err := os.RemoveAll(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) && first == nil {
			first = err
		}
	}
	return first
}

func (s *Service) clearLoginService() error {
	if strings.TrimSpace(s.DataDir) == "" {
		return nil
	}
	home, err := paths.Resolve("")
	if err != nil || filepath.Clean(s.DataDir) != filepath.Clean(home) {
		return nil
	}
	return supervise.Apply(supervise.Spec{DataDir: s.DataDir})
}

func removeEmptyParents(dir string, roots []string) {
	parent := filepath.Dir(filepath.Clean(dir))
	for parent != "" && parent != "." && nestedInRoot(parent, roots) {
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(parent); err != nil {
			return
		}
		parent = filepath.Dir(parent)
	}
}

func nestedInRoot(dir string, roots []string) bool {
	dir = filepath.Clean(dir)
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || dir == root {
			continue
		}
		if isPathPrefix(root, dir) {
			return true
		}
	}
	return false
}

// removeItemDirs deletes captured item folders. A path is skipped when it is
// the archive root, a parent of an archive root, or outside every archive root.
func removeItemDirs(dirs, roots []string) error {
	var first error
	for _, dir := range dirs {
		if !itemDirSafe(dir, roots) {
			continue
		}
		err := os.RemoveAll(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) && first == nil {
			first = err
		}
	}
	return first
}

func itemDirSafe(dir string, roots []string) bool {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" || dir == "." || !filepath.IsAbs(dir) {
		return false
	}
	home, err := os.UserHomeDir()
	if err == nil && dir == filepath.Clean(home) {
		return false
	}
	inside := false
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || root == "." {
			continue
		}
		if dir == root || isPathPrefix(dir, root) {
			return false
		}
		if isPathPrefix(root, dir) {
			inside = true
		}
	}
	return inside
}

func isPathPrefix(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}
