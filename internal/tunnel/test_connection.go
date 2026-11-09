package tunnel

import (
	"context"
	"errors"
	"strings"
	"time"
	"tunnelix/internal/askpass"
	"tunnelix/internal/config"
)

var errTestPassed = errors.New("test passed")
var errTestCancelled = errors.New("test cancelled")

// StartTest runs the actual forwarding configuration once, then releases its ports.
// Successful forwarding setup does not establish destination-service health.
func (m *Manager) StartTest(t config.Tunnel, path string) (string, error) {
	return m.startTest(t, path, 2*time.Minute)
}
func (m *Manager) StartTestWithSecret(t config.Tunnel, path, secret string) (string, error) {
	return m.startTestSecret(t, path, 2*time.Minute, secret)
}
func (m *Manager) startTest(t config.Tunnel, path string, timeout time.Duration) (string, error) {
	return m.startTestSecret(t, path, timeout, "")
}
func (m *Manager) startTestSecret(t config.Tunnel, path string, timeout time.Duration, secret string) (string, error) {
	t.ID = "test-" + askpass.RandomID()
	if strings.TrimSpace(t.Name) == "" {
		t.Name = t.Host
	}
	t.Reconnect = false
	t.AutoConnect = false
	if e := config.ValidateTunnel(t); e != nil {
		return "", e
	}
	exe, e := Executable(path)
	if e != nil {
		return "", e
	}
	m.mu.Lock()
	if m.closed || m.restarting {
		m.mu.Unlock()
		return "", errors.New("APP_CLOSED")
	}
	for id := range m.sessions {
		if m.statuses[id].IsTest {
			m.mu.Unlock()
			return "", errors.New("TEST_IN_PROGRESS")
		}
	}
	deadline, stopDeadline := context.WithTimeout(context.Background(), timeout)
	ctx, cancel := context.WithCancelCause(deadline)
	s := &session{cancel: func() { cancel(errTestCancelled) }, done: make(chan struct{}), secrets: map[string]string{}}
	s.initialSecret = secret
	m.sessions[t.ID] = s
	m.statuses[t.ID] = Status{ID: t.ID, IsTest: true, State: "testing", Logs: []Log{}}
	m.mu.Unlock()
	go func() {
		defer func() {
			cancel(nil)
			stopDeadline()
			m.mu.Lock()
			delete(m.sessions, t.ID)
			clear(s.secrets)
			s.initialSecret = ""
			close(s.done)
			m.mu.Unlock()
			m.notify()
		}()
		m.update(t.ID, "testing", "", 1)
		code, _ := m.run(ctx, t, exe, s, 1, func() { cancel(errTestPassed) })
		if code == "AUTH_FAILED" && s.initialSecret != "" {
			code = "SAVED_AUTH_FAILED"
		}
		switch context.Cause(ctx) {
		case errTestPassed:
			m.update(t.ID, "testPassed", "", 1)
		case errTestCancelled:
			m.update(t.ID, "testCancelled", "", 1)
		case context.DeadlineExceeded:
			m.update(t.ID, "error", "TEST_TIMEOUT", 1)
		default:
			if code == "" {
				code = "CONNECTION_LOST"
			}
			m.update(t.ID, "error", code, 1)
		}
	}()
	return t.ID, nil
}

// CancelTest cannot stop ordinary saved tunnel sessions, even with a supplied ID.
func (m *Manager) CancelTest(id string, forget bool) {
	m.mu.Lock()
	status, ok := m.statuses[id]
	m.mu.Unlock()
	if !ok || !status.IsTest {
		return
	}
	if forget {
		m.Forget(id)
	} else {
		m.Disconnect(id)
	}
}
