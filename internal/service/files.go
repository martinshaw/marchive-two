package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/store"
)

// ItemFile is one file saved with a capture item.
type ItemFile struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Bytes int64  `json:"bytes"`
}

// ItemMessage is the header and files for one capture item.
type ItemMessage struct {
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	URL        string     `json:"url,omitempty"`
	Kind       string     `json:"kind"`
	SourceName string     `json:"sourceName,omitempty"`
	CapturedAt string     `json:"capturedAt,omitempty"`
	Files      []ItemFile `json:"files"`
}

// ItemMessage lists the files stored for one capture item.
func (s *Service) ItemMessage(id int64) (ItemMessage, error) {
	item, err := s.Store.GetItem(id)
	if err != nil {
		return ItemMessage{}, err
	}
	msg := messageFrom(item)
	if src, err := s.Store.GetSource(item.SourceID); err == nil {
		msg.SourceName = src.Name
	}
	dir, err := s.itemDir(item)
	if err != nil {
		return ItemMessage{}, err
	}
	if dir == "" {
		msg.Files = []ItemFile{}
		return msg, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ItemMessage{}, err
	}
	files := make([]ItemFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, ItemFile{Name: entry.Name(), Kind: fileKind(entry.Name()), Bytes: info.Size()})
	}
	sort.Slice(files, func(i, j int) bool {
		pi, pj := fileRank(files[i].Name), fileRank(files[j].Name)
		if pi != pj {
			return pi < pj
		}
		return files[i].Name < files[j].Name
	})
	msg.Files = files
	return msg, nil
}

// ItemFilePath returns a file inside a capture item's folder.
func (s *Service) ItemFilePath(id int64, name string) (string, error) {
	item, err := s.Store.GetItem(id)
	if err != nil {
		return "", err
	}
	dir, err := s.itemDir(item)
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", &Error{Status: 404, Msg: "this item has no files"}
	}
	base, err := safeFileName(name)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, base)
	root := dir
	if linked, err := filepath.EvalSymlinks(dir); err == nil {
		root = linked
	}
	resolved := path
	if linked, err := filepath.EvalSymlinks(path); err == nil {
		resolved = linked
	}
	if !insideAny([]string{root}, resolved) {
		return "", &Error{Status: 400, Msg: "invalid file"}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", &Error{Status: 404, Msg: "that file is not in this item"}
	}
	return resolved, nil
}

func messageFrom(item store.Item) ItemMessage {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "Untitled"
	}
	at := ""
	if !item.CreatedAt.IsZero() {
		at = item.CreatedAt.UTC().Format(time.RFC3339)
	}
	return ItemMessage{
		ID:         item.ID,
		Title:      title,
		URL:        item.URL,
		Kind:       item.Kind,
		CapturedAt: at,
		Files:      []ItemFile{},
	}
}

func (s *Service) itemDir(item store.Item) (string, error) {
	dir := strings.TrimSpace(item.DownloadLocation)
	if dir == "" {
		return "", nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", nil
	}
	roots, err := s.Store.DownloadRoots()
	if err != nil {
		return "", err
	}
	if !insideAny(roots, abs) {
		return "", &Error{Status: 400, Msg: "that folder is not one of your archive folders"}
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", nil
	}
	return abs, nil
}

func safeFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	base := filepath.Base(name)
	if name == "" || base != name || base == "." || base == ".." || strings.ContainsAny(base, `/\`) {
		return "", &Error{Status: 400, Msg: "invalid file"}
	}
	return base, nil
}

func fileKind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt":
		return "text"
	case ".html", ".htm":
		return "html"
	case ".json":
		return "json"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return "image"
	case ".mp3", ".m4a", ".aac", ".ogg", ".wav", ".opus":
		return "audio"
	case ".mp4", ".webm", ".mov", ".m4v":
		return "video"
	case ".mhtml", ".mht":
		return "mhtml"
	default:
		return "file"
	}
}

func fileRank(name string) int {
	switch strings.ToLower(name) {
	case "article.html":
		return 0
	case "text.txt":
		return 1
	case "comments.txt":
		return 2
	case "site.txt":
		return 3
	case "page.html":
		return 4
	case "screenshot.jpg":
		return 5
	case "metadata.json":
		return 6
	case "words.json":
		return 7
	default:
		switch fileKind(name) {
		case "audio", "video", "image":
			return 8
		case "mhtml":
			return 9
		default:
			return 10
		}
	}
}
