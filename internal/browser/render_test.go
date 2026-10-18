package browser

import (
	"testing"
	"time"
)

func TestRenderSettlesAfterThePageStopsChanging(t *testing.T) {
	var wait renderWait
	start := time.Unix(0, 0)
	quiet := renderSample{Ready: true}
	if wait.settled(quiet, start) {
		t.Fatal("first look settled immediately")
	}
	if wait.settled(quiet, start.Add(100*time.Millisecond)) {
		t.Fatal("quiet stretch settled before it was long enough")
	}
	if !wait.settled(quiet, start.Add(100*time.Millisecond).Add(renderQuiet)) {
		t.Fatal("quiet page did not settle")
	}
}

func TestRenderWaitsForRequestsMarkupAndImages(t *testing.T) {
	var wait renderWait
	start := time.Unix(0, 0)
	steps := []renderSample{
		{Ready: false},
		{Ready: true, Inflight: 2},
		{Ready: true, Mutations: 4},
		{Ready: true, Mutations: 4, ImagesPending: true},
		{Ready: true, Mutations: 5},
		{Ready: true, Mutations: 5},
	}
	for i, sample := range steps {
		if wait.settled(sample, start.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("settled at step %d: %+v", i, sample)
		}
	}
	settledAt := start.Add(time.Duration(len(steps)-1) * time.Second).Add(renderQuiet)
	if !wait.settled(steps[len(steps)-1], settledAt) {
		t.Fatal("stable page did not settle after the quiet stretch")
	}
}
