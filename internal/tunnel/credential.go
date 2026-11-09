package tunnel

import (
	"bytes"
	"context"
	"strings"
	"time"
	"tunnelix/internal/config"
	"tunnelix/internal/processguard"
)

// Resolve the final target using OpenSSH itself, including HostName/User aliases.
// Never send a saved target password to an unrelated jump-host prompt.
func (m *Manager) passwordPrompt(ctx context.Context, t config.Tunnel, exe string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := Arguments(t)
	args = append(args[:len(args)-1], "-G", t.Host)
	p, e := processguard.New(m.helper, exe, args, environment(nil))
	if e != nil {
		return ""
	}
	defer p.Close()
	var output bytes.Buffer
	p.Cmd.Stdout = &output
	if p.Start() != nil {
		return ""
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			p.Stop()
		case <-done:
		}
	}()
	if p.Wait() != nil {
		return ""
	}
	var host, user string
	for _, line := range strings.Split(output.String(), "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if key == "hostname" {
			host = strings.TrimSpace(value)
		}
		if key == "user" {
			user = strings.TrimSpace(value)
		}
	}
	if host == "" || user == "" {
		return ""
	}
	return user + "@" + host + "'s password:"
}
