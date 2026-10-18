// Package client calls a running Marchive server.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/martinshaw/marchive-two/internal/service"
)

// Client talks to the localhost API.
type Client struct {
	Base string
	HTTP *http.Client
}

func New(base string) *Client {
	return &Client{
		Base: strings.TrimRight(base, "/"),
		HTTP: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) State() (service.State, error) {
	var state service.State
	err := c.get("/api/state", &state)
	return state, err
}

func (c *Client) CreateSource(in service.SourceInput) (int64, int64, error) {
	var out struct {
		ID         int64 `json:"id"`
		ScheduleID int64 `json:"scheduleId"`
	}
	err := c.post("/api/sources", in, &out)
	return out.ID, out.ScheduleID, err
}

func (c *Client) DeleteSource(id int64) error {
	return c.send(http.MethodDelete, fmt.Sprintf("/api/sources/%d", id), nil, nil)
}

func (c *Client) UpdateSchedule(id int64, in service.ScheduleInput) error {
	return c.send(http.MethodPatch, fmt.Sprintf("/api/schedules/%d", id), in, nil)
}

func (c *Client) RunNow(id int64) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	err := c.post(fmt.Sprintf("/api/schedules/%d/run", id), map[string]any{}, &out)
	return out.ID, err
}

func (c *Client) SetRunner(on bool) error {
	return c.post("/api/runner", map[string]bool{"enabled": on}, nil)
}

func (c *Client) UpdateSettings(in service.SettingsInput) error {
	return c.post("/api/settings", in, nil)
}

func (c *Client) CancelRun(id int64) error {
	return c.post(fmt.Sprintf("/api/runs/%d/cancel", id), map[string]any{}, nil)
}

func (c *Client) SignIn() error {
	return c.post("/api/browser/signin", map[string]any{}, nil)
}

func (c *Client) get(path string, dest any) error {
	return c.send(http.MethodGet, path, nil, dest)
}

func (c *Client) post(path string, body, dest any) error {
	return c.send(http.MethodPost, path, body, dest)
}

func (c *Client) send(method, path string, body, dest any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.Base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var wrapped struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(payload, &wrapped)
		if wrapped.Error == "" {
			wrapped.Error = strings.TrimSpace(string(payload))
		}
		if wrapped.Error == "" {
			wrapped.Error = resp.Status
		}
		return fmt.Errorf("%s", wrapped.Error)
	}
	if dest == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, dest)
}
