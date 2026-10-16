package browser

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFetchExample(t *testing.T) {
	if _, err := Find(); err != nil {
		t.Skip(err)
	}
	client := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	doc, err := client.Fetch(ctx, "https://example.com/", Options{Screenshot: true, MHTML: true, Scroll: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.HTML, "Example Domain") {
		t.Fatalf("unexpected page: %.200q", doc.HTML)
	}
	if len(doc.Screenshot) == 0 {
		t.Fatal("missing screenshot")
	}
	if !strings.Contains(doc.MHTML, "Example Domain") {
		t.Fatal("missing mhtml snapshot")
	}
}
