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
	// terminalGrace is how long a session survives after the page stops talking to
	// it. The dashboard heartbeats every 30 s while its tab is open, so leaving
	// the terminal page (or refreshing it) pauses the clock instead of killing
	// the shell, and coming back inside this window reattaches to it.
	terminalGrace = 5 * time.Minute
	// terminalSweep is how often stale sessions are reaped.
	terminalSweep       = 30 * time.Second
	maxSessionsPerAgent = 2
	defaultTerminalCols = 80
	defaultTerminalRows = 24
	// terminalChunks buffers one stream's channel. A consumer that falls further
	// behind than this is dropped rather than allowed to stall the shell.
	terminalChunks = 64
	// detachedOutputLimit is how many bytes of output are kept while no stream is
	// attached, so a reload does not lose what the shell printed meanwhile.
	detachedOutputLimit = 256 * 1024
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
	// Exactly one goroutine reads ptmx, so the shell's output is never split
	// between two consumers. Subscribers receive copies of what it reads, and
	// output produced while nobody is attached is kept in pending.
	subs    map[*TerminalStream]struct{}
	pending []byte
}

// TerminalStream is one HTTP consumer of a session. A session hands over from an
// old stream to a new one when the page reattaches, which is how a reload takes
// the shell back without racing the reader it left behind.
type TerminalStream struct {
	service *TerminalService
	session *terminalSession
	ch      chan []byte
	closed  bool
}

// Output yields the PTY bytes as they arrive. The channel closes when the stream
// is superseded by a reattach, dropped for falling behind, or the shell dies.
func (t *TerminalStream) Output() <-chan []byte {
	return t.ch
}

// Close detaches this stream. It is safe to call more than once.
func (t *TerminalStream) Close() {
	t.service.mu.Lock()
	defer t.service.mu.Unlock()
	t.closeLocked()
}

func (t *TerminalStream) closeLocked() {
	if t.closed {
		return
	}
	t.closed = true
	delete(t.session.subs, t)
	close(t.ch)
	// The grace window starts when the last consumer goes away.
	t.session.lastUse = time.Now()
}

// TerminalService owns the pseudo-terminals exposed to the dashboard. A session
// runs as the same OS user as this process, in the agent's home directory. It
// outlives a page navigation or reload for a short grace window so the browser
// can reattach, and is reaped once nothing has touched it for that long.
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
	s.reapStaleLocked()
	active := 0
	for _, session := range s.sessions {
		if session.agent == agentName {
			active++
		}
	}
	if active >= maxSessionsPerAgent {
		// A detached session is a shell nobody is watching: the page either
		// reattaches it by ID or it is abandoned. Let a new connection take the
		// slot, oldest detached first.
		if !s.evictDetachedLocked(agentName, active-maxSessionsPerAgent+1) {
			s.mu.Unlock()
			return TerminalInfo{}, ErrTerminalLimit
		}
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
		subs:    make(map[*TerminalStream]struct{}),
	}

	s.mu.Lock()
	s.sessions[session.id] = session
	s.mu.Unlock()

	go s.pump(session)

	// The shell can exit on its own (exit, Ctrl+D). Reap it and drop the session
	// so every attached stream sees the channel close and the client stops.
	go func() {
		_ = cmd.Wait()
		_ = ptmx.Close()
		s.mu.Lock()
		if current, ok := s.sessions[session.id]; ok && current == session {
			s.forgetLocked(session)
		}
		s.mu.Unlock()
	}()

	return TerminalInfo{
		ID:    session.id,
		Shell: shell,
		Cwd:   cwd,
		Cols:  cols,
		Rows:  rows,
	}, nil
}

// pump is the only reader of the PTY. Streams consume copies, so a reattach can
// never race a reader the previous page left blocked on Read.
func (s *TerminalService) pump(session *terminalSession) {
	buffer := make([]byte, 32*1024)
	for {
		read, err := session.ptmx.Read(buffer)
		if read > 0 {
			chunk := make([]byte, read)
			copy(chunk, buffer[:read])
			s.broadcast(session, chunk)
		}
		if err != nil {
			return
		}
	}
}

func (s *TerminalService) broadcast(session *terminalSession, chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(session.subs) == 0 {
		// Nobody is watching: keep the tail so a reattach does not lose it.
		session.pending = append(session.pending, chunk...)
		if over := len(session.pending) - detachedOutputLimit; over > 0 {
			session.pending = append([]byte(nil), session.pending[over:]...)
		}
		return
	}

	session.pending = nil
	for stream := range session.subs {
		select {
		case stream.ch <- chunk:
		default:
			// A stalled consumer must not block the shell; drop it instead.
			stream.closeLocked()
		}
	}
}

// Attach registers a stream for the session and hands it anything the shell
// printed while nobody was watching. Only one stream reads at a time: attaching
// supersedes whatever a reload left behind.
func (s *TerminalService) Attach(agentName, id string) (*TerminalStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, err := s.lookupLocked(agentName, id)
	if err != nil {
		return nil, err
	}

	stream := &TerminalStream{
		service: s,
		session: session,
		ch:      make(chan []byte, terminalChunks),
	}
	if len(session.pending) > 0 {
		stream.ch <- session.pending
		session.pending = nil
	}
	for older := range session.subs {
		older.closeLocked()
	}
	session.subs[stream] = struct{}{}

	return stream, nil
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

// Info describes a live session and refreshes its clock, which is how the page
// reattaches after a reload or a trip to another dashboard page.
func (s *TerminalService) Info(agentName, id string) (TerminalInfo, error) {
	session, err := s.lookup(agentName, id)
	if err != nil {
		return TerminalInfo{}, err
	}
	size, sizeErr := pty.GetsizeFull(session.ptmx)
	info := TerminalInfo{
		ID:    session.id,
		Shell: session.shell,
		Cwd:   session.cwd,
		Cols:  defaultTerminalCols,
		Rows:  defaultTerminalRows,
	}
	if sizeErr == nil {
		info.Cols, info.Rows = size.Cols, size.Rows
	}
	return info, nil
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
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok || session.agent != agentName {
		return nil
	}
	s.forgetLocked(session)
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
	return s.lookupLocked(agentName, id)
}

// lookupLocked resolves a session and refreshes its clock. The caller must hold
// s.mu.
func (s *TerminalService) lookupLocked(agentName, id string) (*terminalSession, error) {
	if !s.Enabled() {
		return nil, ErrTerminalDisabled
	}
	session, ok := s.sessions[id]
	if !ok || session.agent != agentName {
		return nil, ErrTerminalNotFound
	}
	session.lastUse = time.Now()
	return session, nil
}

// sweepIdle reaps stale sessions in the background. Without it a leaked session
// would only be noticed the next time someone opened a shell.
func (s *TerminalService) sweepIdle() {
	ticker := time.NewTicker(terminalSweep)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		s.reapStaleLocked()
		s.mu.Unlock()
	}
}

// evictDetachedLocked drops up to count shells that no stream is watching,
// oldest first, so a fresh connection does not fail while abandoned sessions sit
// in their grace window. The caller must hold s.mu.
func (s *TerminalService) evictDetachedLocked(agentName string, count int) bool {
	for count > 0 {
		var oldest *terminalSession
		for _, session := range s.sessions {
			if session.agent != agentName || len(session.subs) > 0 {
				continue
			}
			if oldest == nil || session.lastUse.Before(oldest.lastUse) {
				oldest = session
			}
		}
		if oldest == nil {
			return false
		}
		s.forgetLocked(oldest)
		count--
	}
	return true
}

// forgetLocked drops a session, releases its streams and terminates its shell.
// The caller must hold s.mu.
func (s *TerminalService) forgetLocked(session *terminalSession) {
	delete(s.sessions, session.id)
	for stream := range session.subs {
		stream.closeLocked()
	}
	if session.cmd.Process != nil {
		_ = session.cmd.Process.Kill()
	}
	_ = session.ptmx.Close()
}

// reapStaleLocked drops sessions that nothing has touched for the grace window,
// which covers crashed tabs and dropped networks. The caller must hold s.mu.
func (s *TerminalService) reapStaleLocked() {
	cutoff := time.Now().Add(-terminalGrace)
	for _, session := range s.sessions {
		if session.lastUse.After(cutoff) {
			continue
		}
		s.forgetLocked(session)
	}
}
