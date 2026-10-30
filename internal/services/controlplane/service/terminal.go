package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agentdir"

	"github.com/creack/pty"
	"github.com/google/uuid"
)

var (
	// ErrTerminalDisabled is returned when the control plane was started without
	// MYAAW_TERMINAL=1.
	ErrTerminalDisabled = errors.New("terminal is disabled; restart the API with `myaaw api restart --terminal`")
	// ErrTerminalNotFound covers both a missing session and one that belongs to
	// another agent.
	ErrTerminalNotFound = errors.New("terminal session not found")
	// ErrTerminalLimit is returned when an agent already has its maximum number
	// of live shells.
	ErrTerminalLimit = errors.New("terminal session limit reached for this agent")
)

const (
	// terminalIdleTimeout is how long a session may go without a heartbeat, an
	// input, or a resize before it is reaped. The dashboard pings every 30 s, so
	// this only catches tabs that vanished without closing their socket.
	terminalIdleTimeout = 2 * time.Minute
	// terminalIdleSweep is how often abandoned sessions are reaped.
	terminalIdleSweep   = 30 * time.Second
	maxSessionsPerAgent = 2
	defaultTerminalCols = 80
	defaultTerminalRows = 24
)

// TerminalInfo is the public description of a live shell.
type TerminalInfo struct {
	ID    string `json:"id"`
	Shell string `json:"shell"`
	Cwd   string `json:"cwd"`
	Cols  uint16 `json:"cols"`
	Rows  uint16 `json:"rows"`
}

type terminalSession struct {
	id      string
	agent   string
	shell   string
	cwd     string
	cmd     *exec.Cmd
	ptmx    *os.File
	lastUse time.Time
}

// TerminalService owns the pseudo-terminals exposed to the dashboard. A session
// runs as the same OS user as this process, in the agent's home directory, and
// dies with its connection: nothing is persisted or daemonised.
type TerminalService struct {
	enabled bool

	mu       sync.Mutex
	sessions map[string]*terminalSession
}

// NewTerminalService reads the enable flag once at startup so the routes can
// stay registered while reporting "disabled".
func NewTerminalService() *TerminalService {
	s := &TerminalService{
		enabled:  strings.TrimSpace(os.Getenv("MYAAW_TERMINAL")) == "1",
		sessions: make(map[string]*terminalSession),
	}
	if s.enabled {
		go s.sweepIdle()
	}
	return s
}

// Enabled reports whether shells may be spawned.
func (s *TerminalService) Enabled() bool {
	return s != nil && s.enabled
}

// Open starts a login shell for the agent in ~/.myaaw/<agent>/home.
func (s *TerminalService) Open(agentName string, cols, rows uint16) (TerminalInfo, error) {
	if !s.Enabled() {
		return TerminalInfo{}, ErrTerminalDisabled
	}
	if err := agentdirValidateAgent(agentName); err != nil {
		return TerminalInfo{}, err
	}
	if cols == 0 {
		cols = defaultTerminalCols
	}
	if rows == 0 {
		rows = defaultTerminalRows
	}

	root, err := agentdir.AgentRoot(agentName)
	if err != nil {
		return TerminalInfo{}, err
	}
	cwd := filepath.Join(root, "home")
	if info, statErr := os.Stat(cwd); statErr != nil || !info.IsDir() {
		cwd = root
	}

	s.mu.Lock()
	s.reapIdleLocked()
	active := 0
	for _, session := range s.sessions {
		if session.agent == agentName {
			active++
		}
	}
	if active >= maxSessionsPerAgent {
		s.mu.Unlock()
		return TerminalInfo{}, ErrTerminalLimit
	}
	s.mu.Unlock()

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell, "-l")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "MYAAW_AGENT="+agentName)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return TerminalInfo{}, fmt.Errorf("start shell: %w", err)
	}

	session := &terminalSession{
		id:      uuid.New().String(),
		agent:   agentName,
		shell:   shell,
		cwd:     cwd,
		cmd:     cmd,
		ptmx:    ptmx,
		lastUse: time.Now(),
	}

	s.mu.Lock()
	s.sessions[session.id] = session
	s.mu.Unlock()

	// The shell can exit on its own (exit, Ctrl+D). Reap it and drop the session
	// so the stream reader sees EOF and the client stops.
	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		if current, ok := s.sessions[session.id]; ok && current == session {
			delete(s.sessions, session.id)
		}
		s.mu.Unlock()
		_ = ptmx.Close()
	}()

	return TerminalInfo{
		ID:    session.id,
		Shell: shell,
		Cwd:   cwd,
		Cols:  cols,
		Rows:  rows,
	}, nil
}

// Reader returns the PTY master for streaming output.
func (s *TerminalService) Reader(agentName, id string) (io.Reader, error) {
	session, err := s.lookup(agentName, id)
	if err != nil {
		return nil, err
	}
	return session.ptmx, nil
}

// Write forwards client input to the shell.
func (s *TerminalService) Write(agentName, id string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	session, err := s.lookup(agentName, id)
	if err != nil {
		return err
	}
	if _, err := session.ptmx.Write(data); err != nil {
		return fmt.Errorf("write terminal: %w", err)
	}
	return nil
}

// Resize applies a new window size so full-screen programs reflow.
func (s *TerminalService) Resize(agentName, id string, cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return nil
	}
	session, err := s.lookup(agentName, id)
	if err != nil {
		return err
	}
	if err := pty.Setsize(session.ptmx, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		return fmt.Errorf("resize terminal: %w", err)
	}
	return nil
}

// Touch refreshes a session's idle clock without writing to the shell. The
// dashboard sends it while its stream is open, so a tab that disappears without
// a clean close is reaped instead of holding one of the agent's slots.
func (s *TerminalService) Touch(agentName, id string) error {
	_, err := s.lookup(agentName, id)
	return err
}

// Close kills the shell. It is idempotent: a missing session is not an error.
func (s *TerminalService) Close(agentName, id string) error {
	s.mu.Lock()
	session, ok := s.sessions[id]
	if !ok || session.agent != agentName {
		s.mu.Unlock()
		return nil
	}
	delete(s.sessions, id)
	s.mu.Unlock()

	if session.cmd.Process != nil {
		_ = session.cmd.Process.Kill()
	}
	_ = session.ptmx.Close()
	return nil
}

// Count returns the number of live sessions, used by tests and diagnostics.
func (s *TerminalService) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *TerminalService) lookup(agentName, id string) (*terminalSession, error) {
	if !s.Enabled() {
		return nil, ErrTerminalDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok || session.agent != agentName {
		return nil, ErrTerminalNotFound
	}
	session.lastUse = time.Now()
	return session, nil
}

// sweepIdle reaps abandoned sessions in the background. Without it a leaked
// session would only be noticed the next time someone opened a shell.
func (s *TerminalService) sweepIdle() {
	ticker := time.NewTicker(terminalIdleSweep)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		s.reapIdleLocked()
		s.mu.Unlock()
	}
}

// reapIdleLocked drops sessions that were never used for too long. The caller
// must hold s.mu.
func (s *TerminalService) reapIdleLocked() {
	cutoff := time.Now().Add(-terminalIdleTimeout)
	for id, session := range s.sessions {
		if session.lastUse.After(cutoff) {
			continue
		}
		delete(s.sessions, id)
		if session.cmd.Process != nil {
			_ = session.cmd.Process.Kill()
		}
		_ = session.ptmx.Close()
	}
}
