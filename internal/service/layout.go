package service

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/martinshaw/marchive-two/internal/provider"
)

// NestSourceCaptures moves item folders that sit directly in an archive root
// into a directory named with the source id and a slug of its name.
func (s *Service) NestSourceCaptures() error {
	items, err := s.Store.ListLocatedItems()
	if err != nil {
		return err
	}
	roots, err := s.Store.SourceArchiveRoots()
	if err != nil {
		return err
	}
	moved := 0
	for _, item := range items {
		dir := filepath.Clean(item.DownloadLocation)
		root := archiveRootFor(dir, roots[item.SourceID])
		if root == "" {
			parent := filepath.Dir(dir)
			if filepath.Base(parent) == provider.SourceDirName(item.SourceID, item.SourceName) {
				continue
			}
			root = parent
		}
		if filepath.Dir(dir) != root {
			continue
		}
		parent := filepath.Join(root, provider.SourceDirName(item.SourceID, item.SourceName))
		dest := filepath.Join(parent, filepath.Base(dir))
		if dest == dir {
			continue
		}
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		if err := os.Rename(dir, dest); err != nil {
			return err
		}
		if err := s.Store.SetItemDownloadLocation(item.ID, dest); err != nil {
			_ = os.Rename(dest, dir)
			return err
		}
		moved++
	}
	if moved > 0 {
		slog.Info("grouped captures by source", "moved", moved)
	}
	return nil
}

func archiveRootFor(dir string, roots []string) string {
	best := ""
	for _, root := range roots {
		root = filepath.Clean(root)
		if root == "" || root == dir || !isPathPrefix(root, dir) {
			continue
		}
		if len(root) > len(best) {
			best = root
		}
	}
	return best
}
