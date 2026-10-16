// Package httpapi serves the local UI and the JSON API on 127.0.0.1.
package httpapi

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/service"
	"github.com/martinshaw/marchive-two/internal/store"
	"github.com/martinshaw/marchive-two/internal/version"
)

//go:embed templates static
var assets embed.FS

func templateTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case time.Time:
		return x, true
	case *time.Time:
		if x == nil {
			return time.Time{}, false
		}
		return *x, true
	default:
		return time.Time{}, false
	}
}

// New returns the localhost handler.
func New(svc *service.Service) http.Handler {
	static, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	h := &handler{
		svc: svc,
		tmpl: template.Must(template.New("").Funcs(template.FuncMap{
			"kind":    provider.Label,
			"version": func() string { return version.Version },
			"every": func(n int, unit string) string {
				if n == 1 && len(unit) > 1 {
					unit = unit[:len(unit)-1]
				}
				return strconv.Itoa(n) + " " + unit
			},
			"stamp": func(v any) string {
				t, ok := templateTime(v)
				if !ok || t.IsZero() {
					return ""
				}
				return t.Local().Format("2 Jan 2006 15:04")
			},
			"unix": func(v any) string {
				t, ok := templateTime(v)
				if !ok || t.IsZero() {
					return ""
				}
				return strconv.FormatInt(t.Unix(), 10)
			},
			"itemMeter": itemMeter,
			"runMeter":  runMeter,
		}).ParseFS(assets, "templates/*.html")),
	}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /{$}", h.page)
	mux.HandleFunc("GET /fragment/live", h.live)
	mux.HandleFunc("GET /fragment/library", h.library)
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/state", h.state)
	mux.HandleFunc("POST /api/sources", h.createSource)
	mux.HandleFunc("POST /sources", h.createSource)
	mux.HandleFunc("DELETE /api/sources/{id}", h.deleteSource)
	mux.HandleFunc("POST /sources/{id}/delete", h.deleteSource)
	mux.HandleFunc("PATCH /api/schedules/{id}", h.updateSchedule)
	mux.HandleFunc("POST /schedules/{id}", h.updateSchedule)
	mux.HandleFunc("DELETE /api/schedules/{id}", h.deleteSchedule)
	mux.HandleFunc("POST /schedules/{id}/delete", h.deleteSchedule)
	mux.HandleFunc("POST /api/schedules/{id}/run", h.runSchedule)
	mux.HandleFunc("POST /schedules/{id}/run", h.runSchedule)
	mux.HandleFunc("POST /api/runner", h.setRunner)
	mux.HandleFunc("POST /runner", h.setRunner)
	mux.HandleFunc("POST /api/runs/{id}/cancel", h.cancelRun)
	mux.HandleFunc("POST /runs/{id}/cancel", h.cancelRun)
	mux.HandleFunc("POST /api/settings", h.settings)
	mux.HandleFunc("POST /settings", h.settings)
	mux.HandleFunc("POST /api/browser/signin", h.signIn)
	mux.HandleFunc("POST /browser/signin", h.signIn)
	mux.HandleFunc("GET /api/chrome", h.chromeStatus)
	mux.HandleFunc("POST /api/chrome/install", h.chromeInstall)
	mux.HandleFunc("POST /api/chrome/install/cancel", h.chromeCancel)
	mux.HandleFunc("POST /api/open", h.openPath)
	mux.HandleFunc("POST /open", h.openPath)
	return guard(mux)
}

type handler struct {
	svc  *service.Service
	tmpl *template.Template
}

func (h *handler) page(w http.ResponseWriter, r *http.Request) {
	state, err := h.svc.State()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state.Error = r.URL.Query().Get("error")
	h.render(w, "page", state)
}

func (h *handler) live(w http.ResponseWriter, r *http.Request) {
	h.fragment(w, r, "live")
}

func (h *handler) library(w http.ResponseWriter, r *http.Request) {
	h.fragment(w, r, "library")
}

func (h *handler) fragment(w http.ResponseWriter, r *http.Request, name string) {
	state, err := h.svc.State()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.render(w, name, state)
}

func (h *handler) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *handler) state(w http.ResponseWriter, r *http.Request) {
	state, err := h.svc.State()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *handler) createSource(w http.ResponseWriter, r *http.Request) {
	var in service.SourceInput
	if wantsJSON(r) {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid JSON"})
			return
		}
	} else {
		if err := parseRequestForm(r); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid form"})
			return
		}
		seconds, err := intervalFromValues(r.FormValue("every_count"), r.FormValue("every_unit"))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		maxNew, err := atoiDefault(r.FormValue("max_new"), 20)
		if err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "new items per run must be a number"})
			return
		}
		enabled := r.FormValue("enabled") == "true"
		capture, err := captureFromForm(r, r.FormValue("provider"))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		in = service.SourceInput{
			Name:             r.FormValue("name"),
			URL:              r.FormValue("url"),
			Provider:         r.FormValue("provider"),
			LinkSelector:     r.FormValue("link_selector"),
			Capture:          capture,
			IntervalSeconds:  seconds,
			DownloadLocation: r.FormValue("download_location"),
			MaxNew:           maxNew,
			Enabled:          &enabled,
		}
	}
	id, scheduleID, err := h.svc.CreateSource(in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if answersJSON(r) {
		writeJSON(w, http.StatusCreated, map[string]int64{"id": id, "scheduleId": scheduleID})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handler) deleteSource(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, &service.Error{Status: 400, Msg: "invalid source"})
		return
	}
	if err := h.svc.DeleteSource(id); err != nil {
		h.fail(w, r, err)
		return
	}
	h.done(w, r, map[string]bool{"ok": true})
}

func (h *handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, &service.Error{Status: 400, Msg: "invalid schedule"})
		return
	}
	var in service.ScheduleInput
	if wantsJSON(r) {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid JSON"})
			return
		}
	} else {
		if err := parseRequestForm(r); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid form"})
			return
		}
		seconds, err := intervalFromValues(r.FormValue("every_count"), r.FormValue("every_unit"))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		maxNew, err := atoiDefault(r.FormValue("max_new"), 20)
		if err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "new items per run must be a number"})
			return
		}
		enabled := r.FormValue("enabled") == "true"
		dest := r.FormValue("download_location")
		capture, err := captureFromForm(r, "")
		if err != nil {
			h.fail(w, r, err)
			return
		}
		in = service.ScheduleInput{
			IntervalSeconds:  &seconds,
			DownloadLocation: &dest,
			MaxNew:           &maxNew,
			Enabled:          &enabled,
			Capture:          &capture,
		}
	}
	if err := h.svc.UpdateSchedule(id, in); err != nil {
		h.fail(w, r, err)
		return
	}
	h.done(w, r, map[string]bool{"ok": true})
}

func (h *handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, &service.Error{Status: 400, Msg: "invalid schedule"})
		return
	}
	if err := h.svc.DeleteSchedule(id); err != nil {
		h.fail(w, r, err)
		return
	}
	h.done(w, r, map[string]bool{"ok": true})
}

func (h *handler) runSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, &service.Error{Status: 400, Msg: "invalid schedule"})
		return
	}
	runID, err := h.svc.RunNow(id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.done(w, r, map[string]int64{"id": runID})
}

func (h *handler) setRunner(w http.ResponseWriter, r *http.Request) {
	var enabled bool
	if wantsJSON(r) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid JSON"})
			return
		}
		enabled = body.Enabled
	} else {
		if err := parseRequestForm(r); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid form"})
			return
		}
		enabled = r.FormValue("enabled") == "true"
	}
	if err := h.svc.SetRunner(enabled); err != nil {
		h.fail(w, r, err)
		return
	}
	h.done(w, r, map[string]bool{"enabled": enabled})
}

func (h *handler) cancelRun(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, &service.Error{Status: 400, Msg: "invalid run"})
		return
	}
	if err := h.svc.CancelRun(id); err != nil {
		h.fail(w, r, err)
		return
	}
	h.done(w, r, map[string]bool{"ok": true})
}

func (h *handler) settings(w http.ResponseWriter, r *http.Request) {
	var in service.SettingsInput
	if wantsJSON(r) {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid JSON"})
			return
		}
	} else {
		if err := parseRequestForm(r); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid form"})
			return
		}
		n, err := strconv.Atoi(r.FormValue("podcast_concurrency"))
		if err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "podcast concurrency must be a number"})
			return
		}
		in.PodcastConcurrency = &n
		switch r.FormValue("port_mode") {
		case "fixed":
			port, err := strconv.Atoi(strings.TrimSpace(r.FormValue("listen_port")))
			if err != nil {
				h.fail(w, r, &service.Error{Status: 400, Msg: "choose a port from 1 to 65535"})
				return
			}
			in.ListenPort = &port
		case "random":
			zero := 0
			in.ListenPort = &zero
		}
		open := formBool(r, "open_on_login")
		restart := formBool(r, "restart_on_crash")
		in.OpenOnLogin = &open
		in.RestartOnCrash = &restart
	}
	if err := h.svc.UpdateSettings(in); err != nil {
		h.fail(w, r, err)
		return
	}
	h.done(w, r, map[string]bool{"ok": true})
}

func (h *handler) signIn(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.BeginSignIn(); err != nil {
		h.fail(w, r, err)
		return
	}
	if answersJSON(r) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "opening Chrome"})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handler) chromeStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.ChromeInstall())
}

func (h *handler) chromeInstall(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.StartChromeInstall(); err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.svc.ChromeInstall())
}

func (h *handler) chromeCancel(w http.ResponseWriter, r *http.Request) {
	h.svc.CancelChromeInstall()
	writeJSON(w, http.StatusOK, h.svc.ChromeInstall())
}

func (h *handler) openPath(w http.ResponseWriter, r *http.Request) {
	var target string
	if wantsJSON(r) {
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid JSON"})
			return
		}
		target = body.Path
	} else {
		if err := parseRequestForm(r); err != nil {
			h.fail(w, r, &service.Error{Status: 400, Msg: "invalid form"})
			return
		}
		target = r.FormValue("path")
	}
	if err := h.svc.OpenPath(target); err != nil {
		h.fail(w, r, err)
		return
	}
	h.done(w, r, map[string]bool{"ok": true})
}

func (h *handler) done(w http.ResponseWriter, r *http.Request, body any) {
	if answersJSON(r) {
		writeJSON(w, http.StatusOK, body)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	var se *service.Error
	switch {
	case errors.As(err, &se):
		status = se.Status
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	default:
		status = http.StatusInternalServerError
	}
	if answersJSON(r) {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	http.Redirect(w, r, "/?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
}

func parseRequestForm(r *http.Request) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return r.ParseMultipartForm(8 << 20)
	}
	return r.ParseForm()
}

func wantsJSON(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/") || strings.Contains(r.Header.Get("Content-Type"), "application/json")
}

func answersJSON(r *http.Request) bool {
	return wantsJSON(r) || strings.Contains(r.Header.Get("Accept"), "application/json")
}

func formBool(r *http.Request, key string) bool {
	vals := r.Form[key]
	if len(vals) == 0 {
		return false
	}
	last := vals[len(vals)-1]
	return last == "1" || last == "true" || last == "on"
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func intervalFromValues(countText, unit string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(countText))
	if err != nil || n < 1 {
		return 0, &service.Error{Status: 400, Msg: "enter how often to check"}
	}
	mult := 1
	switch unit {
	case "hours":
		mult = 3600
	case "minutes":
		mult = 60
	case "seconds":
		mult = 1
	default:
		return 0, &service.Error{Status: 400, Msg: "choose seconds, minutes, or hours"}
	}
	return n * mult, nil
}

func atoiDefault(raw string, fallback int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}

func captureFromForm(r *http.Request, kind string) (string, error) {
	if kind == "" {
		kind = r.FormValue("provider")
	}
	raw, err := provider.EncodeSelection(kind, r.FormValue("capture_all") == "true", r.Form["capture"])
	if err != nil {
		return "", &service.Error{Status: 400, Msg: err.Error()}
	}
	return raw, nil
}

func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "cross-site request blocked", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
