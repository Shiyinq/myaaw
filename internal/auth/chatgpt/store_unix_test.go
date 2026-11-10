//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package chatgpt

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSpecialCredentialFilesNeverBlock(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	if err := os.Remove(m.accountPath(old.ID)); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(m.accountPath(old.ID), 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := m.Get(old.ID)
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("FIFO accepted as a credential file")
		}
	case <-time.After(time.Second):
		t.Fatal("opening an unsafe credential file blocked")
	}
}
