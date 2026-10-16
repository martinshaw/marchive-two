package provider

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWatchIdleCancelsSilentDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &activityReader{r: strings.NewReader("")}
	stop := watchIdle(ctx, cancel, body, 20*time.Millisecond)
	defer stop()
	deadline := time.Now().Add(2 * time.Second)
	for ctx.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if ctx.Err() == nil {
		t.Fatal("expected the idle watch to cancel")
	}
}

func TestActivityReaderCountsBytes(t *testing.T) {
	body := &activityReader{r: strings.NewReader("audio")}
	buf, err := io.ReadAll(body)
	if err != nil || string(buf) != "audio" || body.since() > time.Second {
		t.Fatalf("buf %q err %v idle %s", buf, err, body.since())
	}
}
