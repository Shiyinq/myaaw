package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentruntime "myaaw/internal/agent/runtime"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"

	"github.com/creack/pty"
	"github.com/google/uuid"
)

var (
	// ErrRuntimeAuthSessionNotFound is returned when an auth PTY session ID does
	// not exist or has already exited.
	ErrRuntimeAuthSessionNotFound = errors.New("runtime auth session not found")
	// ErrRuntimeAuthSessionLimit is returned when the maximum number of concurrent
	// interactive auth PTY sessions is already open.
	ErrRuntimeAuthSessionLimit = errors.New("maximum concurrent runtime login sessions reached")
	// ErrRuntimeBinaryMissing is returned when login/logout is requested for a CLI
	// that is not installed yet.
	ErrRuntimeBinaryMissing = errors.New("runtime CLI binary is not installed yet")
)

const (
	maxRuntimeAuthSessions = 2
	runtimeAuthGrace       = 5 * time.Minute
)

// RuntimeAuthSessionInfo describes a scoped interactive PTY session running an
// official CLI login or logout subcommand.
type RuntimeAuthSessionInfo struct {
	ID             string `json:"id"`
	Preset         string `json:"preset"`
	Mode           string `json:"mode"`
	CommandPreview string `json:"command_preview"`
	Cols           uint16 `json:"cols"`
	Rows           uint16 `json:"rows"`
}

type runtimeAuthSession struct {
	id      string
	preset  string
	mode    string
	preview string
	workDir string
	cmd     *exec.Cmd
	ptmx    *os.File
	lastUse time.Time
	subs    map[*RuntimeAuthStream]struct{}
	pending []byte
	exited  bool
}

// RuntimeAuthStream yields raw PTY output bytes for one attached HTTP client.
type RuntimeAuthStream struct {
	service *RuntimeAuthPTYService
	session *runtimeAuthSession
	ch      chan []byte
	closed  bool
}

// Output returns the channel of PTY output chunks. Closed when the CLI exits or
// when a newer client reattaches.
func (s *RuntimeAuthStream) Output() <-chan []byte {
	return s.ch
}

// Close detaches this stream consumer.
func (s *RuntimeAuthStream) Close() {
	s.service.mu.Lock()
	defer s.service.mu.Unlock()
	s.closeLocked()
}

func (s *RuntimeAuthStream) closeLocked() {
	if s.closed {
		return
	}
	s.closed = true
	delete(s.session.subs, s)
	close(s.ch)
	s.session.lastUse = time.Now()
}

// RuntimeAuthPTYService manages short-lived pseudo-terminals strictly scoped to
// official runtime CLI login/logout binaries (never an arbitrary login shell).
type RuntimeAuthPTYService struct {
	mgr      *RuntimeManageService
	mu       sync.Mutex
	sessions map[string]*runtimeAuthSession
}

// NewRuntimeAuthPTYService creates a scoped auth PTY manager.
func NewRuntimeAuthPTYService(mgr *RuntimeManageService) *RuntimeAuthPTYService {
	svc := &RuntimeAuthPTYService{
		mgr:      mgr,
		sessions: make(map[string]*runtimeAuthSession),
	}
	go svc.sweepIdle()
	return svc
}

// Open spawns the allowlisted official login or logout CLI command inside a PTY.
func (s *RuntimeAuthPTYService) Open(presetID, mode string, headless bool, cols, rows uint16) (RuntimeAuthSessionInfo, error) {
	if s == nil || s.mgr == nil || !s.mgr.Enabled() {
		return RuntimeAuthSessionInfo{}, ErrRuntimeManagementDisabled
	}
	id := strings.ToLower(strings.TrimSpace(presetID))
	preset, ok := config.LookupRuntimePreset(id)
	if !ok || preset.ID == config.RuntimeTypeMyaaw {
		return RuntimeAuthSessionInfo{}, ErrRuntimeNotFound
	}

	defaultBin, args, preview, err := agentruntime.AuthCommandArgs(preset.ID, mode, headless)
	if err != nil {
		return RuntimeAuthSessionInfo{}, err
	}

	cmdName := defaultBin
	if saved, hasSaved := s.mgr.RawDefaults(preset.ID); hasSaved && strings.TrimSpace(saved.Command) != "" {
		cmdName = strings.TrimSpace(saved.Command)
	}
	resolvedBin, err := agentruntime.ResolveBinary(cmdName)
	if err != nil {
		return RuntimeAuthSessionInfo{}, fmt.Errorf("%w: %s", ErrRuntimeBinaryMissing, cmdName)
	}

	if cols == 0 {
		cols = defaultTerminalCols
	}
	if rows == 0 {
		rows = defaultTerminalRows
	}

	s.mu.Lock()
	s.reapStaleLocked()
	if len(s.sessions) >= maxRuntimeAuthSessions {
		if !s.evictDetachedLocked() {
			s.mu.Unlock()
			return RuntimeAuthSessionInfo{}, ErrRuntimeAuthSessionLimit
		}
	}
	s.mu.Unlock()

	workDir, err := ensureRuntimeAuthDir(preset.ID)
	if err != nil {
		return RuntimeAuthSessionInfo{}, fmt.Errorf("prepare runtime auth dir: %w", err)
	}

	cmd := exec.Command(resolvedBin, args...)
	cmd.Dir = workDir
	env := append([]string(nil), os.Environ()...)
	env = append(env, "TERM=xterm-256color", "PATH="+agentruntime.EnrichedPATH())
	cmd.Env = env

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return RuntimeAuthSessionInfo{}, fmt.Errorf("start %s: %w", preview, err)
	}

	action := strings.ToLower(strings.TrimSpace(mode))
	if action == "" {
		action = "login"
	}

	sess := &runtimeAuthSession{
		id:      uuid.New().String(),
		preset:  preset.ID,
		mode:    action,
		preview: preview,
		workDir: workDir,
		cmd:     cmd,
		ptmx:    ptmx,
		lastUse: time.Now(),
		subs:    make(map[*RuntimeAuthStream]struct{}),
	}

	s.mu.Lock()
	s.sessions[sess.id] = sess
	s.mu.Unlock()

	go s.pump(sess)
	go func() {
		_ = cmd.Wait()
		// Allow pump() to drain any final buffered PTY bytes before tearing down.
		time.Sleep(80 * time.Millisecond)
		_ = ptmx.Close()
		s.mu.Lock()
		sess.exited = true
		if current, exists := s.sessions[sess.id]; exists && current == sess {
			s.forgetLocked(sess)
		}
		s.mu.Unlock()
	}()

	return RuntimeAuthSessionInfo{
		ID:             sess.id,
		Preset:         sess.preset,
		Mode:           sess.mode,
		CommandPreview: sess.preview,
		Cols:           cols,
		Rows:           rows,
	}, nil
}

func (s *RuntimeAuthPTYService) pump(sess *runtimeAuthSession) {
	buf := make([]byte, 32*1024)
	for {
		n, err := sess.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.broadcast(sess, chunk)
		}
		if err != nil {
			return
		}
	}
}

func (s *RuntimeAuthPTYService) broadcast(sess *runtimeAuthSession, chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(sess.subs) == 0 {
		sess.pending = append(sess.pending, chunk...)
		if over := len(sess.pending) - detachedOutputLimit; over > 0 {
			sess.pending = append([]byte(nil), sess.pending[over:]...)
		}
		return
	}
	sess.pending = nil
	for sub := range sess.subs {
		select {
		case sub.ch <- chunk:
		default:
			sub.closeLocked()
		}
	}
}

// Attach connects a streaming HTTP response to an open auth PTY session.
func (s *RuntimeAuthPTYService) Attach(presetID, sid string) (*RuntimeAuthStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, err := s.lookupLocked(presetID, sid)
	if err != nil {
		return nil, err
	}
	stream := &RuntimeAuthStream{
		service: s,
		session: sess,
		ch:      make(chan []byte, terminalChunks),
	}
	if len(sess.pending) > 0 {
		stream.ch <- sess.pending
		sess.pending = nil
	}
	for older := range sess.subs {
		older.closeLocked()
	}
	sess.subs[stream] = struct{}{}
	return stream, nil
}

// Write forwards interactive user input (arrow keys, Enter, pasted codes) into the PTY.
func (s *RuntimeAuthPTYService) Write(presetID, sid string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	s.mu.Lock()
	sess, err := s.lookupLocked(presetID, sid)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if _, err := sess.ptmx.Write(data); err != nil {
		return fmt.Errorf("write auth pty: %w", err)
	}
	return nil
}

// Resize updates the PTY window dimensions.
func (s *RuntimeAuthPTYService) Resize(presetID, sid string, cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return nil
	}
	s.mu.Lock()
	sess, err := s.lookupLocked(presetID, sid)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return pty.Setsize(sess.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// Close terminates the auth PTY session and cleans up its temporary directory.
func (s *RuntimeAuthPTYService) Close(presetID, sid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sid]
	if !ok || sess.preset != strings.ToLower(strings.TrimSpace(presetID)) {
		return nil
	}
	s.forgetLocked(sess)
	return nil
}

func (s *RuntimeAuthPTYService) lookupLocked(presetID, sid string) (*runtimeAuthSession, error) {
	if s == nil || s.mgr == nil || !s.mgr.Enabled() {
		return nil, ErrRuntimeManagementDisabled
	}
	sess, ok := s.sessions[sid]
	if !ok || sess.preset != strings.ToLower(strings.TrimSpace(presetID)) {
		return nil, ErrRuntimeAuthSessionNotFound
	}
	sess.lastUse = time.Now()
	return sess, nil
}

func (s *RuntimeAuthPTYService) evictDetachedLocked() bool {
	var oldest *runtimeAuthSession
	for _, sess := range s.sessions {
		if len(sess.subs) > 0 {
			continue
		}
		if oldest == nil || sess.lastUse.Before(oldest.lastUse) {
			oldest = sess
		}
	}
	if oldest == nil {
		return false
	}
	s.forgetLocked(oldest)
	return true
}

func (s *RuntimeAuthPTYService) forgetLocked(sess *runtimeAuthSession) {
	delete(s.sessions, sess.id)
	agentruntime.InvalidateAuthStatusCache(sess.preset)
	for sub := range sess.subs {
		sub.closeLocked()
	}
	if sess.cmd != nil && sess.cmd.Process != nil && !sess.exited {
		_ = sess.cmd.Process.Kill()
	}
	if sess.ptmx != nil {
		_ = sess.ptmx.Close()
	}
}

func ensureRuntimeAuthDir(presetID string) (string, error) {
	if dbPath, err := agentdir.ControlPlaneDBPath(); err == nil && dbPath != "" {
		dir := filepath.Join(filepath.Dir(dbPath), "runtime-auth", presetID)
		if mkErr := os.MkdirAll(dir, 0o700); mkErr == nil {
			return dir, nil
		}
	}
	dir := filepath.Join(os.TempDir(), "myaaw-runtime-auth", presetID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func (s *RuntimeAuthPTYService) sweepIdle() {
	ticker := time.NewTicker(terminalSweep)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		s.reapStaleLocked()
		s.mu.Unlock()
	}
}

func (s *RuntimeAuthPTYService) reapStaleLocked() {
	cutoff := time.Now().Add(-runtimeAuthGrace)
	for _, sess := range s.sessions {
		if sess.lastUse.After(cutoff) {
			continue
		}
		s.forgetLocked(sess)
	}
}
