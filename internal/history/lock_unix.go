//go:build darwin || linux

package history

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// flock is released by the OS on process exit, including abnormal termination.
func lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(time.Second)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if (!errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN)) || time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("cannot lock history: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
