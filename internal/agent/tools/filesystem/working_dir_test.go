package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"myaaw/internal/agent/tools"
)

func TestFileSystemToolConcurrentWorkingDirIsolation(t *testing.T) {
	home := t.TempDir()
	agentRoot := filepath.Join(home, ".myaaw", "agents", "default")
	agentHome := filepath.Join(agentRoot, "home")
	if err := os.MkdirAll(agentHome, 0o755); err != nil {
		t.Fatalf("mkdir agent home: %v", err)
	}
	configureAllowedDirectories(agentRoot, agentHome)

	dirA := filepath.Join(home, "proj-a")
	dirB := filepath.Join(home, "proj-b")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatalf("mkdir dirA: %v", err)
	}
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatalf("mkdir dirB: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "note.txt"), []byte("alpha-only"), 0o644); err != nil {
		t.Fatalf("write dirA: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "note.txt"), []byte("beta-only"), 0o644); err != nil {
		t.Fatalf("write dirB: %v", err)
	}

	fsTool := NewFileSystemTool()
	ctxA := &tools.ToolsContext{WorkingDir: dirA}
	ctxB := &tools.ToolsContext{WorkingDir: dirB}

	var wg sync.WaitGroup
	errCh := make(chan string, 40)

	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			readArgs, _ := json.Marshal(map[string]any{
				"tool_name":  "read_file",
				"path":       "note.txt",
				"start_line": 1,
				"end_line":   5,
			})
			out := fsTool.CallTool(string(readArgs), ctxA)
			if !strings.Contains(out, "alpha-only") {
				errCh <- "ctxA read mismatch: " + out
			}
			// Must NOT be allowed to read dirB from ctxA.
			crossArgs, _ := json.Marshal(map[string]any{
				"tool_name":  "read_file",
				"path":       filepath.Join(dirB, "note.txt"),
				"start_line": 1,
				"end_line":   5,
			})
			crossOut := fsTool.CallTool(string(crossArgs), ctxA)
			if !strings.Contains(crossOut, "Security error") {
				errCh <- "ctxA cross-dir should be blocked: " + crossOut
			}
		}()
		go func() {
			defer wg.Done()
			readArgs, _ := json.Marshal(map[string]any{
				"tool_name":  "read_file",
				"path":       "note.txt",
				"start_line": 1,
				"end_line":   5,
			})
			out := fsTool.CallTool(string(readArgs), ctxB)
			if !strings.Contains(out, "beta-only") {
				errCh <- "ctxB read mismatch: " + out
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
}
