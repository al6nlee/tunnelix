package tunnel

import (
	"bufio"
	"context"
	"errors"
	"math/rand/v2"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"tunnelix/internal/askpass"
	"tunnelix/internal/config"
	"tunnelix/internal/processguard"
)

type Status struct {
	IsTest  bool   `json:"isTest"`
	Command string `json:"command"`
	ID      string `json:"id"`
	State   string `json:"state"`
	Code    string `json:"code"`
	Attempt int    `json:"attempt"`
	Since   string `json:"since"`
	Logs    []Log  `json:"logs"`
}
type Log struct {
	Time string `json:"time"`
	Code string `json:"code"`
}
type Challenge struct {
	ID       string `json:"id"`
	TunnelID string `json:"tunnelId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Prompt   string `json:"prompt"`
}
type answer struct {
	value    string
	accepted bool
}
type pending struct {
	Challenge
	response chan answer
	created  time.Time
}
type session struct {
	cancel         context.CancelFunc
	done           chan struct{}
	secrets        map[string]string
	wake           chan struct{}
	interrupt      context.CancelFunc
	recover        bool
	initialSecret  string
	expectedPrompt string
}
type Manager struct {
	mu         sync.Mutex
	sessions   map[string]*session
	statuses   map[string]Status
	pending    map[string]pending
	helper     string
	changed    func()
	closed     bool
	restarting bool
}

func New(helper string) *Manager {
	return &Manager{sessions: map[string]*session{}, statuses: map[string]Status{}, pending: map[string]pending{}, helper: helper}
}
func (m *Manager) OnChange(fn func()) { m.mu.Lock(); m.changed = fn; m.mu.Unlock() }
func (m *Manager) notify() {
	m.mu.Lock()
	fn := m.changed
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}
func (m *Manager) update(id, state, code string, attempt int) {
	m.mu.Lock()
	m.updateLocked(id, state, code, attempt)
	m.mu.Unlock()
	m.notify()
}

// RecordError makes preflight failures visible, including unattended auto-connect.
func (m *Manager) RecordError(id, code string) {
	m.mu.Lock()
	if m.closed || m.sessions[id] != nil {
		m.mu.Unlock()
		return
	}
	m.updateLocked(id, "error", code, 0)
	m.mu.Unlock()
	m.notify()
}
func (m *Manager) updateLocked(id, state, code string, attempt int) {
	s := m.statuses[id]
	s.ID = id
	s.State = state
	s.Code = code
	s.Attempt = attempt
	s.Since = time.Now().UTC().Format(time.RFC3339)
	event := state
	if code != "" {
		event = code
	}
	s.Logs = append(s.Logs, Log{Time: s.Since, Code: event})
	if len(s.Logs) > 100 {
		s.Logs = s.Logs[len(s.Logs)-100:]
	}
	m.statuses[id] = s
}
func (m *Manager) Snapshot() ([]Status, []Challenge) {
	m.mu.Lock()
	defer m.mu.Unlock()
	statuses := []Status{}
	challenges := []Challenge{}
	for _, s := range m.statuses {
		s.Logs = append([]Log{}, s.Logs...)
		statuses = append(statuses, s)
	}
	ordered := make([]pending, 0, len(m.pending))
	for _, p := range m.pending {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].created.Before(ordered[j].created) })
	for _, p := range ordered {
		challenges = append(challenges, p.Challenge)
	}
	return statuses, challenges
}
func (m *Manager) Active(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[id]
	return ok
}

func (m *Manager) HasActiveSessions() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions) > 0
}

// ClearHistory removes inactive statuses when the active configuration changes.
// The service emits the configuration change notification after committing it.
func (m *Manager) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.statuses {
		if m.sessions[id] == nil {
			delete(m.statuses, id)
		}
	}
}
func (m *Manager) Connect(t config.Tunnel, path string) error {
	return m.ConnectWithSecret(t, path, "")
}
func (m *Manager) ConnectWithSecret(t config.Tunnel, path, secret string) error {
	if e := config.ValidateTunnel(t); e != nil {
		return e
	}
	exe, e := Executable(path)
	if e != nil {
		return e
	}
	m.mu.Lock()
	if m.closed || m.restarting {
		m.mu.Unlock()
		return errors.New("APP_CLOSED")
	}
	if _, ok := m.sessions[t.ID]; ok {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{cancel: cancel, done: make(chan struct{}), secrets: map[string]string{}, wake: make(chan struct{}, 1), recover: t.Reconnect}
	s.initialSecret = secret
	m.sessions[t.ID] = s
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.sessions, t.ID)
			clear(s.secrets)
			s.initialSecret = ""
			close(s.done)
			m.mu.Unlock()
			m.notify()
		}()
		for attempt := 1; ; attempt++ {
			if ctx.Err() != nil {
				m.update(t.ID, "disconnected", "", attempt)
				return
			}
			m.update(t.ID, "connecting", "", attempt)
			attemptCtx, stopAttempt := context.WithCancel(ctx)
			m.mu.Lock()
			s.interrupt = stopAttempt
			m.mu.Unlock()
			code, ready := m.run(attemptCtx, t, exe, s, attempt, nil)
			interrupted := attemptCtx.Err() != nil
			stopAttempt()
			m.mu.Lock()
			s.interrupt = nil
			m.mu.Unlock()
			if ctx.Err() != nil {
				m.update(t.ID, "disconnected", "", attempt)
				return
			}
			if interrupted {
				code = "CONNECTION_LOST"
			}
			if code == "" {
				code = "CONNECTION_LOST"
			}
			if code == "AUTH_FAILED" && s.initialSecret != "" {
				code = "SAVED_AUTH_FAILED"
			}
			if fatal(code) || !t.Reconnect {
				m.update(t.ID, "error", code, attempt)
				return
			}
			if ready {
				attempt = 1
			}
			m.update(t.ID, "reconnecting", code, attempt)
			delay := time.Duration(1<<min(attempt, 5))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
			select {
			case <-ctx.Done():
				m.update(t.ID, "disconnected", "", attempt)
				return
			case <-s.wake:
			case <-time.After(delay):
			}
		}
	}()
	return nil
}
func (m *Manager) Disconnect(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	if s != nil {
		s.cancel()
	}
	m.mu.Unlock()
	if s != nil {
		<-s.done
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	list := []*session{}
	for _, s := range m.sessions {
		s.cancel()
		list = append(list, s)
	}
	m.mu.Unlock()
	for _, s := range list {
		<-s.done
	}
}
func (m *Manager) Forget(id string) {
	m.Disconnect(id)
	m.mu.Lock()
	delete(m.statuses, id)
	m.mu.Unlock()
	m.notify()
}
func (m *Manager) Respond(id, value string, accepted bool) error {
	if len(value) > 8192 || strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("INVALID_RESPONSE")
	}
	m.mu.Lock()
	p, ok := m.pending[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("PROMPT_EXPIRED")
	}
	select {
	case p.response <- answer{value, accepted}:
		delete(m.pending, id)
		m.mu.Unlock()
		m.notify()
		return nil
	default:
		m.mu.Unlock()
		return errors.New("PROMPT_EXPIRED")
	}
}
func (m *Manager) prompt(ctx context.Context, t config.Tunnel, s *session, prompt, hint string) (string, error) {
	kind, e := askpass.Kind(prompt, hint)
	if e != nil {
		return "", e
	}
	if s.initialSecret != "" && kind == "password" && s.expectedPrompt != "" && strings.TrimSpace(prompt) == s.expectedPrompt {
		return s.initialSecret, nil
	}
	// Cache only exact prompts within this tunnel session, never host trust decisions.
	m.mu.Lock()
	cached, ok := s.secrets[prompt]
	m.mu.Unlock()
	if kind != "host" && ok {
		return cached, nil
	}
	p := pending{Challenge: Challenge{ID: askpass.RandomID(), TunnelID: t.ID, Name: t.Name, Kind: kind, Prompt: prompt}, response: make(chan answer, 1), created: time.Now()}
	m.mu.Lock()
	m.pending[p.ID] = p
	m.mu.Unlock()
	m.notify()
	defer func() { m.mu.Lock(); delete(m.pending, p.ID); m.mu.Unlock(); m.notify() }()
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("AUTH_TIMEOUT")
		}
		return "", errors.New("AUTH_CANCELLED")
	case a := <-p.response:
		if !a.accepted {
			return "", errors.New("AUTH_CANCELLED")
		}
		if kind == "host" {
			return "yes", nil
		}
		m.mu.Lock()
		s.secrets[prompt] = a.value
		m.mu.Unlock()
		return a.value, nil
	}
}
func (m *Manager) run(ctx context.Context, t config.Tunnel, exe string, s *session, attempt int, onReady func()) (string, bool) {
	if s.initialSecret != "" && t.Auth == "password" {
		s.expectedPrompt = m.passwordPrompt(ctx, t, exe)
	}
	var errorMu sync.Mutex
	code := ""
	ready := false
	setCode := func(v string) {
		errorMu.Lock()
		if !fatal(code) {
			code = v
		}
		errorMu.Unlock()
	}
	broker, e := askpass.Start(ctx, func(c context.Context, p, h string) (string, error) {
		v, e := m.prompt(c, t, s, p, h)
		if e != nil {
			setCode(e.Error())
		}
		return v, e
	})
	if e != nil {
		return "AUTH_BRIDGE_FAILED", false
	}
	defer broker.Close()
	args := Arguments(t)
	m.mu.Lock()
	status := m.statuses[t.ID]
	status.Command = CommandPreview(exe, args, runtime.GOOS)
	m.statuses[t.ID] = status
	m.mu.Unlock()
	m.notify()
	process, e := processguard.New(m.helper, exe, args, environment(broker.Env(m.helper)))
	if e != nil {
		return "PROCESS_FAILED", false
	}
	defer process.Close()
	cmd := process.Cmd
	stderr, e := cmd.StderrPipe()
	if e != nil {
		process.Stop()
		return "PROCESS_FAILED", false
	}
	if e = process.Start(); e != nil {
		return "PROCESS_FAILED", false
	}
	defer process.Stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			process.Stop()
		case <-done:
		}
	}()
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		entered := false
		remoteSuccess := 0
		remoteTotal := 0
		for _, f := range t.Mappings {
			if f.Type == "remote" {
				remoteTotal++
			}
		}
		for scanner.Scan() {
			line := scanner.Text()
			if v := classify(line); v != "" {
				setCode(v)
			}
			if strings.Contains(line, "Entering interactive session.") {
				entered = true
			}
			if strings.Contains(line, "remote forward success for:") {
				remoteSuccess++
			}
			if !ready && entered && remoteSuccess >= remoteTotal {
				errorMu.Lock()
				noError := code == ""
				errorMu.Unlock()
				if noError {
					ready = true
					if onReady != nil {
						onReady()
					} else {
						m.update(t.ID, "connected", "", attempt)
					}
				}
			}
		}
	}()
	<-scanDone
	_ = process.Wait()
	errorMu.Lock()
	defer errorMu.Unlock()
	return code, ready
}

// RecoverNetwork restarts only sessions still requested by the user. It is used
// after system resume; stopped tunnels and one-shot configuration tests stay stopped.
func (m *Manager) RecoverNetwork() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		if m.statuses[id].IsTest || !s.recover {
			continue
		}
		if s.interrupt != nil {
			s.interrupt()
		}
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// ReserveRestart prevents a new session racing the idle check and updater shutdown.
func (m *Manager) ReserveRestart() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.restarting {
		return errors.New("APP_CLOSED")
	}
	if len(m.sessions) > 0 {
		return errors.New("UPDATE_ACTIVE_CONNECTIONS")
	}
	m.restarting = true
	return nil
}
func (m *Manager) ReleaseRestart() { m.mu.Lock(); m.restarting = false; m.mu.Unlock() }
