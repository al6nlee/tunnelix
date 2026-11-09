package tunnel

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"tunnelix/internal/config"
)

func Executable(path string) (string, error) {
	if path == "" {
		path = "ssh"
	}
	p, e := exec.LookPath(path)
	if e != nil {
		return "", errors.New("SSH_NOT_FOUND")
	}
	return filepath.Abs(p)
}
func Arguments(t config.Tunnel) []string {
	network := t.EffectiveNetwork()
	args := []string{"-v", "-N", "-T", "-o", "ExitOnForwardFailure=yes", "-o", "StrictHostKeyChecking=ask", "-o", "BatchMode=no", "-o", "NumberOfPasswordPrompts=1", "-o", "KbdInteractiveAuthentication=no", "-o", "ConnectTimeout=" + strconv.Itoa(network.ConnectTimeout), "-o", "ServerAliveInterval=" + strconv.Itoa(network.ServerAliveInterval), "-o", "ServerAliveCountMax=" + strconv.Itoa(network.ServerAliveCountMax), "-o", "ControlMaster=no", "-o", "ControlPath=none", "-o", "PermitLocalCommand=no", "-o", "RemoteCommand=none", "-o", "ForkAfterAuthentication=no", "-o", "StdinNull=yes", "-o", "LogLevel=DEBUG1"}
	if t.Port > 0 {
		args = append(args, "-p", strconv.Itoa(t.Port))
	}
	if t.User != "" {
		args = append(args, "-l", t.User)
	}
	if t.Auth == "password" {
		args = append(args, "-o", "PreferredAuthentications=password", "-o", "PasswordAuthentication=yes", "-o", "PubkeyAuthentication=no")
	}
	if t.Auth == "key" {
		path := t.Identity
		if strings.HasPrefix(path, "~/") {
			if h, e := os.UserHomeDir(); e == nil {
				path = filepath.Join(h, path[2:])
			}
		}
		args = append(args, "-i", path, "-o", "IdentitiesOnly=yes")
	}
	if t.Jump != "" {
		args = append(args, "-J", t.Jump)
	}
	for _, m := range t.Mappings {
		bind := net.JoinHostPort(m.Bind, strconv.Itoa(m.Port))
		switch m.Type {
		case "dynamic":
			args = append(args, "-D", bind)
		case "local":
			args = append(args, "-L", bind+":"+net.JoinHostPort(m.Target, strconv.Itoa(m.TargetPort)))
		case "remote":
			args = append(args, "-R", bind+":"+net.JoinHostPort(m.Target, strconv.Itoa(m.TargetPort)))
		}
	}
	return append(args, t.Host)
}
func environment(overrides []string) []string {
	env := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "TUNNELIX_ASKPASS") || key == "SSH_ASKPASS" || key == "SSH_ASKPASS_REQUIRE" || key == "SSH_ASKPASS_PROMPT" || key == "DISPLAY" || key == "LANG" || key == "LC_ALL" {
			continue
		}
		env = append(env, entry)
	}
	return append(append(env, "LANG=C", "LC_ALL=C"), overrides...)
}
func classify(line string) string {
	s := strings.ToLower(line)
	switch {
	case strings.Contains(s, "remote host identification has changed"), strings.Contains(s, "host key verification failed"), strings.Contains(s, "offending "):
		return "HOST_KEY_FAILED"
	case strings.Contains(s, "permission denied"), strings.Contains(s, "authentication failed"), strings.Contains(s, "incorrect passphrase"):
		return "AUTH_FAILED"
	case strings.Contains(s, "address already in use"), strings.Contains(s, "cannot listen to port"):
		return "PORT_IN_USE"
	case strings.Contains(s, "remote port forwarding failed"):
		return "REMOTE_FORWARD_FAILED"
	case strings.Contains(s, "could not resolve hostname"):
		return "DNS_FAILED"
	case strings.Contains(s, "connection refused"):
		return "CONNECTION_REFUSED"
	case strings.Contains(s, "no route to host"), strings.Contains(s, "network is unreachable"):
		return "NETWORK_UNREACHABLE"
	case strings.Contains(s, "connection timed out"), strings.Contains(s, "operation timed out"):
		return "CONNECTION_TIMEOUT"
	case strings.Contains(s, "tunnelix: process supervision failed"):
		return "PROCESS_FAILED"
	case strings.Contains(s, "bad configuration option"), strings.Contains(s, "bad configuration options"), strings.Contains(s, "bad owner or permissions"), strings.Contains(s, "no such identity"), strings.Contains(s, "unsupported option"):
		return "SSH_CONFIG_ERROR"
	}
	return ""
}
func fatal(code string) bool {
	switch code {
	case "SAVED_AUTH_FAILED", "PROCESS_FAILED", "AUTH_BRIDGE_FAILED", "AUTH_FAILED", "HOST_KEY_FAILED", "SSH_CONFIG_ERROR", "UNSUPPORTED_AUTH_PROMPT", "AUTH_CANCELLED", "AUTH_TIMEOUT":
		return true
	}
	return false
}
