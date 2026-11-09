package service

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"tunnelix/internal/config"
	"tunnelix/internal/credentials"
	"tunnelix/internal/tunnel"
)

type Snapshot struct {
	Credentials  map[string]credentials.Info `json:"credentials"`
	Tests        []tunnel.Status             `json:"tests"`
	Config       config.Config               `json:"config"`
	Statuses     []tunnel.Status             `json:"statuses"`
	Challenges   []tunnel.Challenge          `json:"challenges"`
	SSHAvailable bool                        `json:"sshAvailable"`
	SSHPath      string                      `json:"sshPath"`
}
type Service struct {
	mu       sync.Mutex
	store    *config.Store
	cfg      config.Config
	manager  *tunnel.Manager
	vault    *credentials.Vault
	revision uint64
	OnChange func()
}

func New(store *config.Store, manager *tunnel.Manager) (*Service, error) {
	c, e := store.Load()
	if e != nil {
		return nil, e
	}
	absolute, _ := filepath.Abs(store.Path())
	return &Service{store: store, cfg: c, manager: manager, vault: credentials.New(absolute)}, nil
}
func (s *Service) notify() {
	s.mu.Lock()
	fn := s.OnChange
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
}
func clone(c config.Config) config.Config {
	b, _ := json.Marshal(c)
	var out config.Config
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	c := clone(s.cfg)
	infos := map[string]credentials.Info{}
	for i := range c.Tunnels {
		if r := c.Tunnels[i].Credential; r != nil {
			infos[c.Tunnels[i].ID] = credentials.Info{Mode: r.Mode, Saved: true}
		}
		c.Tunnels[i].Credential = nil
	}
	s.mu.Unlock()
	statuses, challenges := s.manager.Snapshot()
	normal, tests := []tunnel.Status{}, []tunnel.Status{}
	for _, status := range statuses {
		if status.IsTest {
			tests = append(tests, status)
		} else {
			normal = append(normal, status)
		}
	}
	path, e := tunnel.Executable(c.Settings.SSHPath)
	return Snapshot{Credentials: infos, Tests: tests, Config: c, Statuses: normal, Challenges: challenges, SSHAvailable: e == nil, SSHPath: path}
}
func credentialScope(t config.Tunnel) string {
	b, _ := json.Marshal([]any{t.ID, t.Host, t.Port, t.User, t.Auth, t.Identity, t.Jump})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (s *Service) SaveTunnel(t config.Tunnel) error {
	return s.SaveTunnelWithCredential(t, credentials.Update{Mode: "keep"})
}
func (s *Service) SaveTunnelWithCredential(t config.Tunnel, input credentials.Update) error {
	t.Credential = nil
	if e := config.ValidateTunnel(t); e != nil {
		return e
	}
	s.mu.Lock()
	if s.manager.Active(t.ID) {
		s.mu.Unlock()
		return errors.New("DISCONNECT_TO_EDIT")
	}
	c := clone(s.cfg)
	var old *config.Tunnel
	for i := range c.Tunnels {
		if c.Tunnels[i].ID == t.ID {
			old = &c.Tunnels[i]
			break
		}
	}
	revision := s.revision
	s.mu.Unlock()
	record, e := s.prepareCredential(t, old, input)
	if e != nil {
		return e
	}
	s.mu.Lock()
	if revision != s.revision {
		s.mu.Unlock()
		return errors.New("CONFIG_CHANGED")
	}
	if s.manager.Active(t.ID) {
		s.mu.Unlock()
		return errors.New("DISCONNECT_TO_EDIT")
	}
	t.Credential = record
	found := false
	for i := range c.Tunnels {
		if c.Tunnels[i].ID == t.ID {
			c.Tunnels[i] = t
			found = true
			break
		}
	}
	if !found {
		c.Tunnels = append(c.Tunnels, t)
	}
	e = s.store.Save(c)
	if e == nil {
		s.cfg = c
		s.revision++
	}
	s.mu.Unlock()
	if e == nil {
		s.notify()
	}
	return e
}
func (s *Service) DeleteTunnel(id string) error {
	s.mu.Lock()
	if s.manager.Active(id) {
		s.mu.Unlock()
		return errors.New("DISCONNECT_TO_EDIT")
	}
	c := clone(s.cfg)
	out := []config.Tunnel{}
	for _, t := range c.Tunnels {
		if t.ID != id {
			out = append(out, t)
		}
	}
	c.Tunnels = out
	e := s.store.Save(c)
	if e == nil {
		s.cfg = c
		s.revision++
	}
	s.mu.Unlock()
	if e == nil {
		s.manager.Forget(id)
		s.notify()
	}
	return e
}
func (s *Service) SaveSettings(settings config.Settings) error {
	s.mu.Lock()
	c := clone(s.cfg)
	c.Settings = settings
	e := s.store.Save(c)
	if e == nil {
		s.cfg = c
		s.revision++
	}
	s.mu.Unlock()
	if e == nil {
		s.notify()
	}
	return e
}
func (s *Service) Connect(id string) (result error) {
	defer func() {
		if result != nil && result.Error() != "TUNNEL_NOT_FOUND" {
			s.manager.RecordError(id, result.Error())
		}
	}()
	s.mu.Lock()
	c := clone(s.cfg)
	revision := s.revision
	s.mu.Unlock()
	for _, t := range c.Tunnels {
		if t.ID == id {
			if s.manager.Active(id) {
				return nil
			}
			secret, e := s.vault.Open(t.Credential, credentialScope(t))
			if e != nil {
				return e
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if revision != s.revision {
				return errors.New("CONFIG_CHANGED")
			}
			t.Credential = nil
			return s.manager.ConnectWithSecret(t, c.Settings.SSHPath, secret)
		}
	}
	return errors.New("TUNNEL_NOT_FOUND")
}
func (s *Service) Disconnect(id string) { s.manager.Disconnect(id) }
func (s *Service) DisconnectAll() {
	snapshot := s.Snapshot()
	for _, t := range snapshot.Config.Tunnels {
		s.manager.Disconnect(t.ID)
	}
}
func (s *Service) Respond(id, value string, accepted bool) error {
	return s.manager.Respond(id, value, accepted)
}
func (s *Service) ExportConfig() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := clone(s.cfg)
	for i := range c.Tunnels {
		c.Tunnels[i].Credential = nil
	}
	b, e := json.MarshalIndent(c, "", "  ")
	return string(b), e
}

// Imports merge tunnels. IDs are intentionally preserved; collisions require explicit deletion first.
func (s *Service) ImportConfig(data string) error {
	if len(data) > 2*1024*1024 {
		return errors.New("CONFIG_TOO_LARGE")
	}
	incoming, e := config.Decode([]byte(data))
	if e != nil {
		return e
	}
	s.mu.Lock()
	c := clone(s.cfg)
	ids := map[string]bool{}
	for _, t := range c.Tunnels {
		ids[t.ID] = true
	}
	for _, t := range incoming.Tunnels {
		if t.Credential != nil {
			s.mu.Unlock()
			return errors.New("IMPORT_CONTAINS_CREDENTIALS")
		}
		if ids[t.ID] {
			s.mu.Unlock()
			return errors.New("DUPLICATE_ID")
		}
		t.AutoConnect = false
		c.Tunnels = append(c.Tunnels, t)
	}
	e = s.store.Save(c)
	if e == nil {
		s.cfg = c
		s.revision++
	}
	s.mu.Unlock()
	if e == nil {
		s.notify()
	}
	return e
}
func (s *Service) AutoConnect() {
	snapshot := s.Snapshot()
	for _, t := range snapshot.Config.Tunnels {
		if t.AutoConnect {
			_ = s.Connect(t.ID)
		}
	}
}

// Aliases enumerates literal Host entries. OpenSSH remains responsible for resolving configuration.
func (s *Service) Aliases() []string {
	home, e := os.UserHomeDir()
	if e != nil {
		return []string{}
	}
	aliases := map[string]bool{}
	seen := map[string]bool{}
	var visit func(string, int)
	visit = func(path string, depth int) {
		if depth > 8 || seen[path] || len(seen) > 128 {
			return
		}
		seen[path] = true
		f, e := os.Open(path)
		if e != nil {
			return
		}
		defer f.Close()
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			line := strings.TrimSpace(strings.SplitN(scan.Text(), "#", 2)[0])
			fields := strings.Fields(strings.ReplaceAll(line, "=", " "))
			if len(fields) < 2 {
				continue
			}
			switch strings.ToLower(fields[0]) {
			case "host":
				for _, a := range fields[1:] {
					a = strings.Trim(a, "\"'")
					if !strings.ContainsAny(a, "*?!") && !strings.HasPrefix(a, "-") {
						aliases[a] = true
					}
				}
			case "include":
				for _, p := range fields[1:] {
					p = strings.Trim(p, "\"'")
					if strings.HasPrefix(p, "~/") {
						p = filepath.Join(home, p[2:])
					}
					if !filepath.IsAbs(p) {
						p = filepath.Join(home, ".ssh", p)
					}
					matches, _ := filepath.Glob(p)
					for _, match := range matches {
						visit(match, depth+1)
					}
				}
			}
		}
	}
	visit(filepath.Join(home, ".ssh", "config"), 0)
	out := []string{}
	for a := range aliases {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// PreviewCommand is also used for unsaved form values. It never starts a process.
func (s *Service) PreviewCommand(t config.Tunnel) (string, error) {
	if e := config.ValidateTunnel(t); e != nil {
		return "", e
	}
	s.mu.Lock()
	path := s.cfg.Settings.SSHPath
	s.mu.Unlock()
	if resolved, e := tunnel.Executable(path); e == nil {
		path = resolved
	} else if path == "" {
		path = "ssh"
	}
	return tunnel.CommandPreview(path, tunnel.Arguments(t), runtime.GOOS), nil
}

// StartTest uses the unsaved draft without writing configuration or enabling retries.
func (s *Service) StartTest(t config.Tunnel) (string, error) {
	return s.TestDraft(t, credentials.Update{Mode: "keep"})
}
func (s *Service) TestDraft(t config.Tunnel, input credentials.Update) (string, error) {
	t.Credential = nil
	if e := config.ValidateTunnel(t); e != nil {
		return "", e
	}
	s.mu.Lock()
	c := clone(s.cfg)
	revision := s.revision
	s.mu.Unlock()
	if s.manager.Active(t.ID) {
		return "", errors.New("DISCONNECT_TO_TEST")
	}
	secret := ""
	switch input.Mode {
	case "none":
		secret = input.Value
	case "keep", "plain", "encrypted":
		secret = input.Value
		if input.Mode == "keep" || secret == "" {
			for _, old := range c.Tunnels {
				if old.ID == t.ID && credentialScope(old) == credentialScope(t) {
					var e error
					secret, e = s.vault.Open(old.Credential, credentialScope(old))
					if e != nil {
						return "", e
					}
					break
				}
			}
		}
		if input.Mode != "keep" && secret == "" {
			return "", errors.New("INVALID_RESPONSE")
		}
	default:
		return "", errors.New("INVALID_CREDENTIAL")
	}
	if secret != "" && (t.Auth != "password" || !credentials.ValidSecret(secret)) {
		return "", errors.New("INVALID_RESPONSE")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision != s.revision {
		return "", errors.New("CONFIG_CHANGED")
	}
	if s.manager.Active(t.ID) {
		return "", errors.New("DISCONNECT_TO_TEST")
	}
	return s.manager.StartTestWithSecret(t, c.Settings.SSHPath, secret)
}
func (s *Service) CancelTest(id string) { s.manager.CancelTest(id, false) }
func (s *Service) ClearTest(id string)  { s.manager.CancelTest(id, true) }

// prepareCredential never accepts credential records supplied by a frontend.
// Re-encryption and configuration changes are committed together by Store.Save.
func (s *Service) prepareCredential(t config.Tunnel, old *config.Tunnel, input credentials.Update) (*credentials.Record, error) {
	switch input.Mode {
	case "none":
		return nil, nil
	case "keep":
		if old != nil && credentialScope(*old) == credentialScope(t) {
			return old.Credential, nil
		}
		return nil, nil
	case "plain", "encrypted":
		if t.Auth != "password" {
			return nil, errors.New("INVALID_CREDENTIAL")
		}
		value := input.Value
		if value == "" && old != nil && credentialScope(*old) == credentialScope(t) && old.Credential != nil {
			var e error
			value, e = s.vault.Open(old.Credential, credentialScope(*old))
			if e != nil {
				return nil, e
			}
		}
		return s.vault.Seal(input.Mode, value, credentialScope(t))
	default:
		return nil, errors.New("INVALID_CREDENTIAL")
	}
}

func (s *Service) ConnectAll() error {
	var first error
	for _, t := range s.Snapshot().Config.Tunnels {
		if e := s.Connect(t.ID); e != nil && first == nil {
			first = e
		}
	}
	return first
}
