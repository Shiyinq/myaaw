package chatgpt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

func privateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return errors.New("cannot create private ChatGPT credential directory")
	}
	root, err := openPrivateDir(path)
	if err != nil {
		return err
	}
	return root.Close()
}

// Pin directories with os.Root: a later directory/symlink swap must not redirect
// token reads or atomic writes outside the private store.
func openPrivateDir(path string) (*os.Root, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || !ownedByUser(info) {
		return nil, errors.New("unsafe ChatGPT credential directory")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, errors.New("cannot open ChatGPT credential directory")
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) || root.Chmod(".", 0o700) != nil {
		root.Close()
		return nil, errors.New("cannot protect ChatGPT credential directory")
	}
	return root, nil
}

func privateSubdir(parent *os.Root, name string) (*os.Root, error) {
	if err := parent.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, errors.New("cannot create private ChatGPT credential directory")
	}
	info, err := parent.Lstat(name)
	if err != nil || !info.IsDir() || !ownedByUser(info) {
		return nil, errors.New("unsafe ChatGPT credential directory")
	}
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, errors.New("cannot open ChatGPT credential directory")
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) || root.Chmod(".", 0o700) != nil {
		root.Close()
		return nil, errors.New("cannot protect ChatGPT credential directory")
	}
	if err := syncRoot(parent); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

// Verify the opened inode, including ownership and hard links, before chmod/read.
func privateFileAt(root *os.Root, name string, flags int) (*os.File, error) {
	if info, err := root.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("unsafe ChatGPT credential file")
	}
	f, err := openStoreFile(root, name, flags)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, errors.New("cannot open ChatGPT credential file")
	}
	info, statErr := f.Stat()
	link, linkErr := root.Lstat(name)
	if statErr != nil || linkErr != nil || !info.Mode().IsRegular() || !safeStoreFile(info) || link.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, link) {
		f.Close()
		return nil, errors.New("unsafe ChatGPT credential file")
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return nil, errors.New("cannot protect ChatGPT credential file")
	}
	return f, nil
}

func readJSONAt(root *os.Root, name string, target any) error {
	f, err := privateFileAt(root, name, os.O_RDONLY)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, target) != nil {
		return errors.New("cannot read ChatGPT credential record")
	}
	return nil
}

func atomicJSON(path string, value any) error {
	root, err := openPrivateDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	return atomicJSONAt(root, filepath.Base(path), value)
}

func atomicJSONAt(root *os.Root, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil || len(data) > 1<<20 {
		return errors.New("cannot encode ChatGPT credential record")
	}
	temp := ".chatgpt-" + randomValue()
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("cannot create ChatGPT credential file")
	}
	defer root.Remove(temp)
	defer f.Close()
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot write ChatGPT credential file")
	}
	// Rename replaces a link itself rather than following its target.
	if err := root.Rename(temp, name); err != nil {
		return errors.New("cannot replace ChatGPT credential file")
	}
	return syncRoot(root)
}

func (m *Manager) lock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := privateFileAt(m.store, ".lock", os.O_CREATE|os.O_RDWR)
	if err != nil {
		return nil, err
	}
	for {
		locked, err := tryLock(f)
		if err != nil {
			f.Close()
			return nil, errors.New("cannot lock ChatGPT credential store")
		}
		if locked {
			if err := ctx.Err(); err != nil {
				unlockFile(f)
				f.Close()
				return nil, err
			}
			return func() { unlockFile(f); f.Close() }, nil
		}
		timer := time.NewTimer(15 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			f.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
