package browser

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareProfileNamesMarchive(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "Local State")
	if err := os.WriteFile(local, []byte(`{"profile":{"picker_shown":true,"info_cache":{"Default":{"name":"Your Chrome","is_using_default_name":true,"gaia_id":9007199254740993}}},"keep":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	prefs := filepath.Join(dir, "Default", "Preferences")
	if err := os.MkdirAll(filepath.Dir(prefs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prefs, []byte(`{"profile":{"name":"Your Chrome"},"keep":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareProfile(dir); err != nil {
		t.Fatal(err)
	}
	if err := prepareProfile(dir); err != nil {
		t.Fatal(err)
	}
	state := readJSON(t, local)
	profile := state["profile"].(map[string]any)
	if profile["picker_shown"] != false {
		t.Fatalf("picker %#v", profile["picker_shown"])
	}
	info := profile["info_cache"].(map[string]any)["Default"].(map[string]any)
	if info["name"] != "Marchive" || info["is_using_default_name"] != false {
		t.Fatalf("info %#v", info)
	}
	if info["gaia_id"].(json.Number).String() != "9007199254740993" || state["keep"] != true {
		t.Fatalf("state %#v", state)
	}
	got := readJSON(t, prefs)
	if got["profile"].(map[string]any)["name"] != "Marchive" || got["keep"] != true {
		t.Fatalf("prefs %#v", got)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
