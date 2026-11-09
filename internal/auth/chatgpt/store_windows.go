package chatgpt

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func openStoreFile(root *os.Root, name string, flags int) (*os.File, error) {
	f, err := root.OpenFile(name, flags, 0o600)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil || info.NumberOfLinks != 1 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		f.Close()
		return nil, errors.New("unsafe ChatGPT credential file")
	}
	return f, nil
}

// Unix ownership/mode checks have no Windows equivalent; the host must provide
// an owner-only ACL on its credential directory.
func ownedByUser(os.FileInfo) bool        { return true }
func safeStoreFile(info os.FileInfo) bool { return info.Mode().IsRegular() }

func tryLock(f *os.File) (bool, error) {
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{})
}

func syncRoot(*os.Root) error { return nil } // File contents are flushed before rename.
