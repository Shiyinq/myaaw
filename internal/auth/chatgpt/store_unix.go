//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package chatgpt

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func openStoreFile(root *os.Root, name string, flags int) (*os.File, error) {
	// Do not follow a swapped symlink or block opening a FIFO before validation.
	return root.OpenFile(name, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
}

func ownedByUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func safeStoreFile(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && ownedByUser(info) && stat.Nlink == 1
}

func tryLock(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }

func syncRoot(root *os.Root) error {
	f, err := root.Open(".")
	if err != nil {
		return errors.New("cannot sync ChatGPT credential directory")
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return errors.New("cannot sync ChatGPT credential directory")
	}
	return nil
}
