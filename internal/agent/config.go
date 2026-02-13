package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EnsureMyaawConfig ensures that the .myaaw configuration directory exists in the user's home directory.
// If it doesn't exist, it copies the default configuration from the current working directory.
// Returns the path to the home .myaaw directory.
func EnsureMyaawConfig() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	targetDir := filepath.Join(homeDir, ".myaaw")

	// Check if target directory exists
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		// Directory already exists, return path
		return targetDir, nil
	}

	// Directory does not exist, copy from current working directory
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	sourceDir := filepath.Join(wd, ".myaaw")

	// Check if source directory exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		// Source .myaaw does not exist in current directory, can't copy
		// Just return the target path, maybe the caller will handle initialization differently
		// Or should we error? The requirement says "copy all contents... if not exists"
		// If source doesn't exist, we can't copy.
		return targetDir, fmt.Errorf("source configuration directory %s not found", sourceDir)
	}

	// Recursive copy
	err = copyDir(sourceDir, targetDir)
	if err != nil {
		return "", fmt.Errorf("failed to copy configuration to home directory: %w", err)
	}

	return targetDir, nil
}

func copyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return fmt.Errorf("source is not a directory")
	}

	err = os.MkdirAll(dst, si.Mode())
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = copyDir(srcPath, dstPath)
			if err != nil {
				return err
			}
		} else {
			err = copyFile(srcPath, dstPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return out.Close()
}
