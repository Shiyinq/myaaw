package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"myaaw/internal/agentdir"
)

const (
	logTailBytes    = 64 << 10
	logPollInterval = 500 * time.Millisecond
)

type LogFile struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

// LogsService lists and tails files under an agent's logs directory.
type LogsService struct{}

func NewLogsService() *LogsService {
	return &LogsService{}
}

func (s *LogsService) List(agentName string) ([]LogFile, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}
	logsDir, err := agentdir.LogsDir(agentName)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogFile{}, nil
		}
		return nil, err
	}

	files := make([]LogFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		files = append(files, LogFile{
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
	return files, nil
}

// Resolve validates a log file name and returns its absolute path.
func (s *LogsService) Resolve(agentName, name string) (string, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return "", err
	}
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return "", ErrInvalidPath
	}

	logsDir, err := agentdir.LogsDir(agentName)
	if err != nil {
		return "", err
	}
	logPath := filepath.Join(logsDir, name)
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrPathNotFound
		}
		return "", err
	}
	if info.IsDir() {
		return "", ErrInvalidPath
	}
	return logPath, nil
}

// Tail sends the last lines of a log file, then polls for new lines until the
// context is canceled.
func (s *LogsService) Tail(ctx context.Context, logPath string, emit func(string) error) error {
	file, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	start := info.Size() - logTailBytes
	if start < 0 {
		start = 0
	}
	pending := ""
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return err
	}
	if info.Size() > start {
		buf := make([]byte, info.Size()-start)
		n, readErr := io.ReadFull(file, buf)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		pending = string(buf[:n])
		if start > 0 {
			// Drop the partial first line from the tail window.
			if idx := strings.IndexByte(pending, '\n'); idx >= 0 {
				pending = pending[idx+1:]
			} else {
				pending = ""
			}
		}
	}

	emitCompleteLines := func() error {
		lines := strings.Split(pending, "\n")
		pending = lines[len(lines)-1]
		for _, line := range lines[:len(lines)-1] {
			if err := emit(line); err != nil {
				return err
			}
		}
		return nil
	}
	if err := emitCompleteLines(); err != nil {
		return err
	}

	offset := info.Size()
	ticker := time.NewTicker(logPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			info, err := file.Stat()
			if err != nil {
				return err
			}

			if info.Size() < offset {
				// The file was truncated; restart from the beginning.
				if _, err := file.Seek(0, io.SeekStart); err != nil {
					return err
				}
				offset = 0
				pending = ""
			}
			if info.Size() == offset {
				continue
			}

			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				return err
			}
			buf := make([]byte, info.Size()-offset)
			n, readErr := io.ReadFull(file, buf)
			offset = info.Size()
			if n > 0 {
				pending += string(buf[:n])
				if err := emitCompleteLines(); err != nil {
					return err
				}
			}
			if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
				return readErr
			}
		}
	}
}
