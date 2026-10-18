package browser

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

const profileName = "Marchive"

// prepareProfile names the Chrome profile Marchive and skips the profile picker.
// The files live in the app data directory, not in the user's normal Chrome profiles.
func prepareProfile(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "Default"), 0o755); err != nil {
		return err
	}
	if err := writeProfileFile(filepath.Join(dir, "Local State"), func(doc map[string]any) {
		profile := mapField(doc, "profile")
		profile["picker_shown"] = false
		info := mapField(mapField(profile, "info_cache"), "Default")
		info["name"] = profileName
		info["is_using_default_name"] = false
	}); err != nil {
		return err
	}
	return writeProfileFile(filepath.Join(dir, "Default", "Preferences"), func(doc map[string]any) {
		mapField(doc, "profile")["name"] = profileName
	})
}

func writeProfileFile(path string, edit func(map[string]any)) error {
	doc := map[string]any{}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			return err
		}
	}
	edit(doc)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func mapField(doc map[string]any, key string) map[string]any {
	if existing, ok := doc[key].(map[string]any); ok {
		return existing
	}
	next := map[string]any{}
	doc[key] = next
	return next
}
