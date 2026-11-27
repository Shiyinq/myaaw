package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"

	"myaaw/internal/workspaceguard"
)

var (
	ErrHostBrowseDisabled = errors.New("host directory browsing is disabled by MYAAW_ALLOW_HOST_BROWSE=0")
	ErrWorkspaceInvalid   = errors.New("invalid workspace directory")
)

// WorkspaceRootItem is a quick-jump shortcut shown in the host directory picker.
type WorkspaceRootItem struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Kind  string `json:"kind"` // "agent_workspace" | "home" | "projects" | "custom"
}

// WorkspaceRootsResponse describes the host filesystem roots available to attach.
type WorkspaceRootsResponse struct {
	Hostname        string              `json:"hostname"`
	OS              string              `json:"os"`
	AllowHostBrowse bool                `json:"allow_host_browse"`
	Roots           []WorkspaceRootItem `json:"roots"`
}

// WorkspaceBrowseEntry is one child directory returned by Browse.
type WorkspaceBrowseEntry struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	IsGit   bool     `json:"is_git"`
	IsEmpty bool     `json:"is_empty"`
	Markers []string `json:"markers,omitempty"`
}

// WorkspaceBrowseResponse is one directory listing for the host folder picker.
type WorkspaceBrowseResponse struct {
	Path      string                 `json:"path"`
	Parent    string                 `json:"parent,omitempty"`
	CanAttach bool                   `json:"can_attach"`
	Entries   []WorkspaceBrowseEntry `json:"entries"`
}

// WorkspaceService lists safe host directories and creates empty folders for
// attaching as per-chat working directories.
type WorkspaceService struct{}

func NewWorkspaceService() *WorkspaceService {
	return &WorkspaceService{}
}

// Roots returns the API host identity and shortcut directories the user can
// browse to attach as a conversation working directory.
func (s *WorkspaceService) Roots(agentName string) (WorkspaceRootsResponse, error) {
	if strings.TrimSpace(agentName) != "" {
		if err := agentdirValidateAgent(agentName); err != nil {
			return WorkspaceRootsResponse{}, err
		}
	}
	hostname, _ := os.Hostname()
	rawRoots := workspaceguard.BrowseRoots(agentName)
	items := make([]WorkspaceRootItem, 0, len(rawRoots))
	for _, r := range rawRoots {
		items = append(items, WorkspaceRootItem{
			Label: r.Label,
			Path:  r.Path,
			Kind:  r.Kind,
		})
	}
	return WorkspaceRootsResponse{
		Hostname:        hostname,
		OS:              goruntime.GOOS,
		AllowHostBrowse: workspaceguard.HostBrowseEnabled(),
		Roots:           items,
	}, nil
}

var workspaceProjectMarkers = []struct {
	file  string
	label string
}{
	{".git", "git"},
	{"go.mod", "go"},
	{"package.json", "node"},
	{"pyproject.toml", "python"},
	{"requirements.txt", "python"},
	{"Cargo.toml", "rust"},
	{"composer.json", "php"},
	{"Gemfile", "ruby"},
	{"pom.xml", "java"},
	{"CMakeLists.txt", "cpp"},
}

// Browse lists subdirectories of targetPath on the API host, filtering out
// sensitive/denied directories so the user only sees safe folders to navigate
// or attach.
func (s *WorkspaceService) Browse(agentName, targetPath string, showHidden bool) (WorkspaceBrowseResponse, error) {
	if !workspaceguard.HostBrowseEnabled() {
		return WorkspaceBrowseResponse{}, ErrHostBrowseDisabled
	}
	if strings.TrimSpace(agentName) != "" {
		if err := agentdirValidateAgent(agentName); err != nil {
			return WorkspaceBrowseResponse{}, err
		}
	}

	raw := strings.TrimSpace(targetPath)
	if raw == "" {
		roots := workspaceguard.BrowseRoots(agentName)
		if len(roots) > 0 {
			raw = roots[0].Path
		} else if home, err := os.UserHomeDir(); err == nil {
			raw = home
		}
	}

	resolved, err := workspaceguard.ValidateBrowseParent(agentName, raw)
	if err != nil {
		return WorkspaceBrowseResponse{}, fmt.Errorf("%w: %v", ErrWorkspaceInvalid, err)
	}

	dirEntries, err := os.ReadDir(resolved)
	if err != nil {
		return WorkspaceBrowseResponse{}, err
	}

	entries := make([]WorkspaceBrowseEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		name := entry.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.IsDir() {
			// Skip regular files; the picker only attaches directories.
			continue
		}
		childPath := filepath.Join(resolved, name)
		if _, vErr := workspaceguard.ValidateDir(agentName, childPath); vErr != nil {
			continue
		}
		isGit := workspaceguard.HasGit(childPath)
		isEmpty, markers := inspectWorkspaceDir(childPath, isGit)
		entries = append(entries, WorkspaceBrowseEntry{
			Name:    name,
			Path:    childPath,
			IsGit:   isGit,
			IsEmpty: isEmpty,
			Markers: markers,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := ""
	candParent := filepath.Dir(resolved)
	if candParent != resolved {
		if _, pErr := workspaceguard.ValidateBrowseParent(agentName, candParent); pErr == nil {
			parent = candParent
		}
	}

	_, attachErr := workspaceguard.ValidateDir(agentName, resolved)

	return WorkspaceBrowseResponse{
		Path:      resolved,
		Parent:    parent,
		CanAttach: attachErr == nil,
		Entries:   entries,
	}, nil
}

// Mkdir creates a new empty directory inside parent so the user can attach a
// fresh folder directly from the picker modal.
func (s *WorkspaceService) Mkdir(agentName, parent, name string) (string, error) {
	if !workspaceguard.HostBrowseEnabled() {
		return "", ErrHostBrowseDisabled
	}
	if err := agentdirValidateAgent(agentName); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:`+"\x00") {
		return "", fmt.Errorf("%w: invalid folder name", ErrWorkspaceInvalid)
	}
	parentResolved, err := workspaceguard.ValidateBrowseParent(agentName, parent)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrWorkspaceInvalid, err)
	}
	validated, err := workspaceguard.ValidateCreatableDir(agentName, parentResolved, name)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrWorkspaceInvalid, err)
	}
	if err := os.MkdirAll(validated, 0o755); err != nil {
		return "", err
	}
	return validated, nil
}

func inspectWorkspaceDir(dir string, isGit bool) (isEmpty bool, markers []string) {
	children, err := os.ReadDir(dir)
	if err != nil {
		return false, nil
	}
	if len(children) == 0 {
		return true, nil
	}
	names := make(map[string]struct{}, len(children))
	for _, c := range children {
		names[c.Name()] = struct{}{}
	}
	seen := make(map[string]struct{})
	if isGit {
		markers = append(markers, "git")
		seen["git"] = struct{}{}
	}
	for _, m := range workspaceProjectMarkers {
		if m.file == ".git" {
			continue
		}
		if _, ok := names[m.file]; ok {
			if _, dup := seen[m.label]; !dup {
				seen[m.label] = struct{}{}
				markers = append(markers, m.label)
			}
		}
	}
	return false, markers
}
