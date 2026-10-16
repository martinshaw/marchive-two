// Package lock keeps a second Marchive process from opening the database.
package lock

import (
	"errors"
	"os"
	"syscall"
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
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Release unlocks and closes the file.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	cerr := l.f.Close()
	l.f = nil
	if err != nil {
		return err
	}
	return cerr
}
