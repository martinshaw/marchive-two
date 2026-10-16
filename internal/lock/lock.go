// Package lock keeps a second Marchive process from opening the database.
package lock

import (
	"errors"
	"os"
)

// ErrBusy means another process holds the data directory.
var ErrBusy = errors.New("marchive is already running")

// Lock is an exclusive advisory lock on a file.
type Lock struct {
	f *os.File
}

// Acquire takes a non-blocking exclusive lock, creating the file if needed.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := flock(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Release unlocks and closes the file.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := funlock(l.f)
	cerr := l.f.Close()
	l.f = nil
	if err != nil {
		return err
	}
	return cerr
}
