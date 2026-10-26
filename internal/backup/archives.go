package backup

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

var archiveNamePattern = regexp.MustCompile(`^myaaw-backup-(.+)-(\d{8}-\d{6})(?:-\d+)?\.tar\.gz$`)

// ArchiveInfo describes an archive found by ListArchives.
type ArchiveInfo struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Agent     string    `json:"agent"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// ListArchives returns the backup archives in dir, newest first.
func ListArchives(dir string) ([]ArchiveInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	archives := make([]ArchiveInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := archiveNamePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		created, err := time.ParseInLocation("20060102-150405", match[2], time.Local)
		if err != nil {
			created = info.ModTime()
		}
		archives = append(archives, ArchiveInfo{
			Path:      filepath.Join(dir, entry.Name()),
			Name:      entry.Name(),
			Agent:     match[1],
			Size:      info.Size(),
			CreatedAt: created,
		})
	}

	sort.Slice(archives, func(i, j int) bool {
		if archives[i].CreatedAt.Equal(archives[j].CreatedAt) {
			return archives[i].Name > archives[j].Name
		}
		return archives[i].CreatedAt.After(archives[j].CreatedAt)
	})
	return archives, nil
}
