package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/martinshaw/marchive-two/internal/provider"
	"github.com/martinshaw/marchive-two/internal/runner"
	"github.com/martinshaw/marchive-two/internal/service"
	"github.com/martinshaw/marchive-two/internal/store"
)

type okProvider struct{}

func (okProvider) Kind() string { return provider.KindWebpage }

func (okProvider) Run(context.Context, provider.Request) (provider.Summary, error) {
	return provider.Summary{Note: "captured"}, nil
}

func TestPageAndCapture(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "marchive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	reg := provider.NewRegistry()
	reg.Register(okProvider{})
	runr := runner.New(st, reg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runr.Start(ctx)
	defer func() {
		cancel()
		runr.Shutdown()
	}()
	svc := service.New(st, runr, nil)
	ts := httptest.NewServer(New(svc))
	defer ts.Close()

	page, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page.Body.Close()
	if page.StatusCode != 200 {
		t.Fatalf("page %d", page.StatusCode)
	}

	body := `{"name":"Desk","url":"https://example.com","provider":"webpage","intervalSeconds":3600,"downloadLocation":"` + filepath.Join(dir, "out") + `","maxNew":5,"enabled":true}`
	res, err := http.Post(ts.URL+"/api/sources", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("create %d", res.StatusCode)
	}
	var created struct {
		ScheduleID int64 `json:"scheduleId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runRes, err := http.Post(ts.URL+"/api/schedules/"+itoa(created.ScheduleID)+"/run", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	runRes.Body.Close()
	if runRes.StatusCode != 200 {
		t.Fatalf("run %d", runRes.StatusCode)
	}

	deadline := time.Now().Add(3 * time.Second)
	var note string
	for time.Now().Before(deadline) {
		state, err := svc.State()
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Runs) > 0 && state.Runs[0].Status == store.StatusSucceeded {
			note = state.Runs[0].Note
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if note != "captured" {
		t.Fatalf("note %q", note)
	}

	blocked, err := http.NewRequest(http.MethodPost, ts.URL+"/api/runner", strings.NewReader(`{"enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	blocked.Header.Set("Content-Type", "application/json")
	blocked.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(blocked)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site %d", resp.StatusCode)
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
