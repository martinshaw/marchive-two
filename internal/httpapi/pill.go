package httpapi

import "github.com/martinshaw/marchive-two/internal/store"

// pillMeter is the fill shown inside a status chip.
type pillMeter struct {
	Status  string
	Percent int
	Busy    bool
}

func itemMeter(status string) pillMeter {
	switch status {
	case store.StatusQueued:
		return pillMeter{Status: status}
	case store.StatusRunning:
		return pillMeter{Status: status, Busy: true}
	default:
		return pillMeter{Status: status, Percent: 100}
	}
}

func runMeter(status string, items []store.Item) pillMeter {
	switch status {
	case store.StatusQueued:
		return pillMeter{Status: status}
	case store.StatusRunning:
		done := 0
		for _, item := range items {
			if item.Status != store.StatusQueued && item.Status != store.StatusRunning {
				done++
			}
		}
		if len(items) == 0 || done == 0 {
			return pillMeter{Status: status, Busy: true}
		}
		return pillMeter{Status: status, Percent: done * 100 / len(items)}
	default:
		return pillMeter{Status: status, Percent: 100}
	}
}
