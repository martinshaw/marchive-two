package service

import (
	"html"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func finderProxy(path string) []byte {
	switch runtime.GOOS {
	case "windows":
		return []byte("[InternetShortcut]\r\nURL=" + fileURL(path) + "\r\n")
	case "linux":
		return []byte("[Desktop Entry]\nType=Link\nURL=" + fileURL(path) + "\n")
	default:
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>URL</key>
	<string>` + html.EscapeString(fileURL(path)) + `</string>
</dict>
</plist>
`)
	}
}

func fileURL(path string) string {
	slash := filepath.ToSlash(path)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String()
}
