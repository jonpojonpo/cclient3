package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	maxHistoryLines = 2000 // lines kept per session
	maxSessions     = 10
	sentinelFmt     = "<<<CCLIENT3_DONE_%016x>>>"
)

// SessionInfo is returned by SessionManager.List.
type SessionInfo struct {
	Name        string
	LastCommand string
	StartedAt   time.Time
	LastUsedAt  time.Time
	Alive       bool
}

// bashSession is a long-lived bash subprocess with a line-oriented output feed.
type bashSession struct {
	mu          sync.Mutex   // guards history, lastCommand, lastUsedAt, observers
	cmdMu       sync.Mutex   // serializes concurrent run calls (sentinel protocol requires it)
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	lines       chan string   // live output lines (buffered, written by pumpLoop)
	history     []string     // ring buffer of last maxHistoryLines lines
	observers   []chan string // registered observers get copies of output lines
	lastCommand string
	startedAt   time.Time
	lastUsedAt  time.Time
	dead        chan struct{} // closed when the subprocess exits
}

func newBashSession() (*bashSession, error) {
	cmd := exec.Command("/bin/bash", "--norc", "--noprofile")
	// Pass through the parent environment so PATH, HOME, etc. are available.
	// Override a few vars that would produce terminal noise.
	cmd.Env = append(os.Environ(), "PS1=", "PS2=", "TERM=dumb", "PAGER=cat")
	// Put the process in its own group so we can kill it cleanly.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start bash: %w", err)
	}

	s := &bashSession{
		cmd:       cmd,
		stdin:     stdinPipe,
		lines:     make(chan string, 4000),
		startedAt: time.Now(),
		dead:      make(chan struct{}),
	}

	// Two goroutines merge stdout and stderr into s.lines.
	var wg sync.WaitGroup
	wg.Add(2)
	readInto := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			s.mu.Lock()
			s.history = append(s.history, line)
			if len(s.history) > maxHistoryLines {
				s.history = s.history[len(s.history)-maxHistoryLines:]
			}
			// Broadcast to observers (non-blocking)
			for _, obs := range s.observers {
				select {
				case obs <- line:
				default:
				}
			}
			s.mu.Unlock()
			select {
			case s.lines <- line:
			default: // channel full — history still has it
			}
		}
	}
	go readInto(stdoutPipe)
	go readInto(stderrPipe)
	go func() {
		wg.Wait()
		close(s.dead)
	}()

	return s, nil
}

// run executes command in the session and returns the output.
//
// If background is true the command is started with & so the sentinel fires
// immediately; the caller can read_session later for output.
//
// totalTimeout=0 means no total deadline.
// idleTimeout=0 means no idle deadline.
// maxLines=0 means return everything.
//
// Returns (output, timedOut).
func (s *bashSession) run(ctx context.Context, command string, background bool, totalTimeout, idleTimeout time.Duration, maxLines int) (string, bool) {
	// Serialize concurrent calls — sentinel protocol breaks if interleaved.
	s.cmdMu.Lock()
	defer s.cmdMu.Unlock()

	s.mu.Lock()
	s.lastCommand = command
	s.lastUsedAt = time.Now()
	s.mu.Unlock()

	sentinel := fmt.Sprintf(sentinelFmt, rand.Int63())

	// Write command + sentinel to stdin.
	var payload string
	if background {
		// background: cmd runs detached, printf fires immediately.
		payload = fmt.Sprintf("%s &\nprintf '\\n%s\\n'\n", command, sentinel)
	} else {
		// foreground: printf fires only after command exits.
		payload = fmt.Sprintf("( %s )\nprintf '\\n%s\\n'\n", command, sentinel)
	}
	if _, err := io.WriteString(s.stdin, payload); err != nil {
		return fmt.Sprintf("write to session: %v", err), true
	}

	// Collect lines until sentinel, timeout, or context cancel.
	var collected []string

	var totalC <-chan time.Time
	var totalTimer *time.Timer
	if totalTimeout > 0 {
		totalTimer = time.NewTimer(totalTimeout)
		defer totalTimer.Stop()
		totalC = totalTimer.C
	}

	var idleC <-chan time.Time
	var idleTimer *time.Timer
	if idleTimeout > 0 {
		idleTimer = time.NewTimer(idleTimeout)
		defer idleTimer.Stop()
		idleC = idleTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			return strings.Join(collected, "\n") + "\n[cancelled — session still running]", true

		case <-s.dead:
			return strings.Join(collected, "\n") + "\n[session process exited]", true

		case <-totalC:
			return strings.Join(collected, "\n") + fmt.Sprintf("\n[total timeout after %v]", totalTimeout), true

		case <-idleC:
			return strings.Join(collected, "\n") + fmt.Sprintf("\n[idle timeout: no output for %v]", idleTimeout), true

		case line := <-s.lines:
			if idleTimer != nil {
				// Safe reset: idleTimer.C is only drained via the select above;
				// since we hold cmdMu this is the only goroutine reading it.
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(idleTimeout)
			}
			if line == sentinel {
				goto done
			}
			collected = append(collected, line)
		}
	}

done:
	if maxLines > 0 && len(collected) > maxLines {
		omitted := len(collected) - maxLines
		tail := collected[len(collected)-maxLines:]
		return fmt.Sprintf("[%d lines omitted]\n", omitted) + strings.Join(tail, "\n"), false
	}
	if len(collected) == 0 {
		return "(no output)", false
	}
	return strings.Join(collected, "\n"), false
}

func (s *bashSession) recentHistory(n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 || n >= len(s.history) {
		return append([]string{}, s.history...)
	}
	return append([]string{}, s.history[len(s.history)-n:]...)
}

func (s *bashSession) isAlive() bool {
	select {
	case <-s.dead:
		return false
	default:
		return true
	}
}

// AddObserver registers a channel to receive copies of all output lines.
func (s *bashSession) AddObserver(ch chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, ch)
}

// RemoveObserver unregisters an observer channel.
func (s *bashSession) RemoveObserver(ch chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, obs := range s.observers {
		if obs == ch {
			s.observers = append(s.observers[:i], s.observers[i+1:]...)
			return
		}
	}
}

// kill terminates the session: closes stdin (signals bash to exit) and
// sends SIGKILL to the entire process group to handle subprocesses.
func (s *bashSession) kill() {
	s.stdin.Close()
	if s.cmd.Process != nil {
		// Negative PID kills the entire process group.
		syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// SessionManager manages named persistent bash sessions.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*bashSession
}

func NewSessionManager() *SessionManager {
	return &SessionManager{sessions: make(map[string]*bashSession)}
}

// acquire returns (or creates) the named session, auto-recreating if dead.
func (sm *SessionManager) acquire(name string) (*bashSession, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if s, ok := sm.sessions[name]; ok && s.isAlive() {
		return s, nil
	}
	delete(sm.sessions, name)

	if len(sm.sessions) >= maxSessions {
		return nil, fmt.Errorf("session limit (%d) reached; kill a session first", maxSessions)
	}

	s, err := newBashSession()
	if err != nil {
		return nil, err
	}
	sm.sessions[name] = s
	return s, nil
}

// Acquire returns (or creates) the named session. Public wrapper around acquire.
func (sm *SessionManager) Acquire(name string) error {
	_, err := sm.acquire(name)
	return err
}

// AddObserver registers an observer on the named session.
// Returns false if the session doesn't exist.
func (sm *SessionManager) AddObserver(name string, ch chan string) bool {
	sm.mu.Lock()
	s, ok := sm.sessions[name]
	sm.mu.Unlock()
	if !ok {
		return false
	}
	s.AddObserver(ch)
	return true
}

// RemoveObserver unregisters an observer from the named session.
func (sm *SessionManager) RemoveObserver(name string, ch chan string) {
	sm.mu.Lock()
	s, ok := sm.sessions[name]
	sm.mu.Unlock()
	if !ok {
		return
	}
	s.RemoveObserver(ch)
}

func (sm *SessionManager) Kill(name string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[name]
	if !ok {
		return false
	}
	s.kill()
	delete(sm.sessions, name)
	return true
}

func (sm *SessionManager) List() []SessionInfo {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	var out []SessionInfo
	for name, s := range sm.sessions {
		s.mu.Lock()
		out = append(out, SessionInfo{
			Name:        name,
			LastCommand: s.lastCommand,
			StartedAt:   s.startedAt,
			LastUsedAt:  s.lastUsedAt,
			Alive:       s.isAlive(),
		})
		s.mu.Unlock()
	}
	return out
}

func (sm *SessionManager) ReadHistory(name string, lines int) ([]string, bool) {
	sm.mu.Lock()
	s, ok := sm.sessions[name]
	sm.mu.Unlock()
	if !ok {
		return nil, false
	}
	return s.recentHistory(lines), true
}

// Prewarm pre-spawns the named sessions in the background so they are ready
// to use immediately on first call (no cold-start bash launch latency).
func (sm *SessionManager) Prewarm(names ...string) {
	for _, name := range names {
		name := name
		go func() {
			_, _ = sm.acquire(name)
		}()
	}
}

// KillAll terminates every active session. Called on agent shutdown.
func (sm *SessionManager) KillAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for name, s := range sm.sessions {
		s.kill()
		delete(sm.sessions, name)
	}
}
