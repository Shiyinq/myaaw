package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"myaaw/internal/agentdir"
)

const (
	maxReadSize  = 2 << 20 // 2 MiB
	maxWriteSize = 2 << 20 // 2 MiB
	maxSearch    = 50
)

var (
	ErrInvalidPath   = errors.New("invalid path")
	ErrPathNotFound  = errors.New("path not found")
	ErrPathTooLarge  = errors.New("file is too large")
	ErrBinaryFile    = errors.New("binary files cannot be displayed")
	ErrRootProtected = errors.New("agent root cannot be modified")
)

// FileEntry describes one directory entry using virtual paths.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

// FileContent is a text file returned to the client.
type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

// SearchMatch is one matching line found under an agent root.
type SearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type agentRoot struct {
	virtual string
	abs     string
}

// FileService exposes the per-agent home, skills, and jobs directories.
type FileService struct{}

func NewFileService() *FileService {
	return &FileService{}
}

// List returns the three virtual roots or the entries of a directory.
func (s *FileService) List(agentName, virtualPath string) ([]FileEntry, error) {
	if virtualPath == "" || virtualPath == "/" {
		return []FileEntry{
			{Name: "home", Path: "/home", IsDir: true},
			{Name: "skills", Path: "/skills", IsDir: true},
			{Name: "jobs", Path: "/jobs", IsDir: true},
		}, nil
	}

	abs, virtual, _, err := s.resolve(agentName, virtualPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrPathNotFound
		}
		return nil, err
	}

	result := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		result = append(result, FileEntry{
			Name:    entry.Name(),
			Path:    path.Join(virtual, entry.Name()),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// Read returns a text file.
func (s *FileService) Read(agentName, virtualPath string) (FileContent, error) {
	abs, virtual, _, err := s.resolve(agentName, virtualPath)
	if err != nil {
		return FileContent{}, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return FileContent{}, ErrPathNotFound
		}
		return FileContent{}, err
	}
	if info.IsDir() {
		return FileContent{}, fmt.Errorf("%w: path is a directory", ErrInvalidPath)
	}
	if info.Size() > maxReadSize {
		return FileContent{}, ErrPathTooLarge
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return FileContent{}, err
	}
	if looksBinary(data) {
		return FileContent{}, ErrBinaryFile
	}

	return FileContent{
		Path:    virtual,
		Content: string(data),
		Size:    info.Size(),
		ModTime: info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// Write creates or overwrites a text file.
func (s *FileService) Write(agentName, virtualPath, content string) (FileContent, error) {
	if len(content) > maxWriteSize {
		return FileContent{}, ErrPathTooLarge
	}

	abs, virtual, _, err := s.resolve(agentName, virtualPath)
	if err != nil {
		return FileContent{}, err
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return FileContent{}, err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return FileContent{}, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return FileContent{}, err
	}
	return FileContent{
		Path:    virtual,
		Content: content,
		Size:    info.Size(),
		ModTime: info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// Mkdir creates a directory and any missing parents.
func (s *FileService) Mkdir(agentName, virtualPath string) error {
	abs, _, _, err := s.resolve(agentName, virtualPath)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

// Rename moves a file or directory inside the same agent root.
func (s *FileService) Rename(agentName, from, to string) error {
	fromAbs, fromVirtual, _, err := s.resolve(agentName, from)
	if err != nil {
		return err
	}
	if isRootPath(fromVirtual) {
		return ErrRootProtected
	}
	toAbs, toVirtual, _, err := s.resolve(agentName, to)
	if err != nil {
		return err
	}
	if isRootPath(toVirtual) {
		return ErrRootProtected
	}
	if _, err := os.Stat(toAbs); err == nil {
		return fmt.Errorf("%w: destination already exists", ErrInvalidPath)
	}
	return os.Rename(fromAbs, toAbs)
}

// Delete removes a file or directory. The three roots cannot be deleted.
func (s *FileService) Delete(agentName, virtualPath string, recursive bool) error {
	abs, virtual, _, err := s.resolve(agentName, virtualPath)
	if err != nil {
		return err
	}
	if isRootPath(virtual) {
		return ErrRootProtected
	}

	if recursive {
		return os.RemoveAll(abs)
	}
	return os.Remove(abs)
}

// Search scans files under a root for a case-insensitive substring.
func (s *FileService) Search(agentName, virtualPath, query string) ([]SearchMatch, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: query must not be empty", ErrInvalidPath)
	}
	if virtualPath == "" {
		virtualPath = "/home"
	}

	abs, virtual, _, err := s.resolve(agentName, virtualPath)
	if err != nil {
		return nil, err
	}

	needle := strings.ToLower(query)
	matches := make([]SearchMatch, 0, maxSearch)

	walkErr := filepath.WalkDir(abs, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if len(matches) >= maxSearch {
			return fs.SkipAll
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil || info.Size() > maxReadSize {
			return nil
		}

		data, readErr := os.ReadFile(filePath)
		if readErr != nil || looksBinary(data) {
			return nil
		}

		rel, relErr := filepath.Rel(abs, filePath)
		if relErr != nil {
			return nil
		}
		matchPath := path.Join(virtual, filepath.ToSlash(rel))

		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(strings.ToLower(line), needle) {
				continue
			}
			matches = append(matches, SearchMatch{
				Path: matchPath,
				Line: i + 1,
				Text: strings.TrimSpace(line),
			})
			if len(matches) >= maxSearch {
				return fs.SkipAll
			}
		}
		return nil
	})
	if walkErr != nil {
		if os.IsNotExist(walkErr) {
			return nil, ErrPathNotFound
		}
		return nil, walkErr
	}
	return matches, nil
}

// resolve maps a virtual path (/home, /skills, /jobs) to a filesystem path and
// rejects traversal, other agents, and symlink escapes.
func (s *FileService) resolve(agentName, virtualPath string) (string, string, agentRoot, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return "", "", agentRoot{}, err
	}

	roots, err := s.roots(agentName)
	if err != nil {
		return "", "", agentRoot{}, err
	}

	if virtualPath == "" {
		virtualPath = "/home"
	}
	if !strings.HasPrefix(virtualPath, "/") {
		virtualPath = "/home/" + virtualPath
	}
	cleaned := path.Clean(virtualPath)
	if cleaned == "/" {
		return "", "", agentRoot{}, fmt.Errorf("%w: choose /home, /skills, or /jobs", ErrInvalidPath)
	}

	for _, root := range roots {
		if cleaned != root.virtual && !strings.HasPrefix(cleaned, root.virtual+"/") {
			continue
		}
		rel := strings.TrimPrefix(cleaned, root.virtual)
		abs := filepath.Join(root.abs, filepath.FromSlash(rel))
		if err := ensureSymlinksInside(root.abs, abs); err != nil {
			return "", "", agentRoot{}, err
		}
		return abs, cleaned, root, nil
	}

	return "", "", agentRoot{}, fmt.Errorf("%w: path must start with /home, /skills, or /jobs", ErrInvalidPath)
}

func (s *FileService) roots(agentName string) ([]agentRoot, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}

	home, err := agentdir.Home(agentName)
	if err != nil {
		return nil, err
	}
	skills, err := agentdir.SkillsDir(agentName)
	if err != nil {
		return nil, err
	}
	jobs, err := agentdir.JobsDir(agentName)
	if err != nil {
		return nil, err
	}

	return []agentRoot{
		{virtual: "/home", abs: home},
		{virtual: "/skills", abs: skills},
		{virtual: "/jobs", abs: jobs},
	}, nil
}

func isRootPath(virtual string) bool {
	return virtual == "/home" || virtual == "/skills" || virtual == "/jobs"
}

func looksBinary(data []byte) bool {
	limit := len(data)
	if limit > 8000 {
		limit = 8000
	}
	for _, b := range data[:limit] {
		if b == 0 {
			return true
		}
	}
	return false
}

// ensureSymlinksInside rejects symlinks that resolve outside the root. For
// paths that do not exist yet, the nearest existing ancestor is checked.
func ensureSymlinksInside(root, abs string) error {
	current := abs
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			if !isWithin(root, resolved) {
				return fmt.Errorf("%w: symlink escapes the agent root", ErrInvalidPath)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func isWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func agentdirValidateAgent(name string) error {
	if err := agentdir.ValidateName(name); err != nil {
		return ErrAgentNotFound
	}
	if !agentdir.AgentInitialized(name) {
		return ErrAgentNotFound
	}
	return nil
}
