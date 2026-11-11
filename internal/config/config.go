package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"tunnelix/internal/credentials"
)

type Mapping struct {
	Type       string `json:"type"`
	Bind       string `json:"bind"`
	Port       int    `json:"port"`
	Target     string `json:"target"`
	TargetPort int    `json:"targetPort"`
}
type Tunnel struct {
	Network     *NetworkOptions     `json:"network,omitempty"`
	Credential  *credentials.Record `json:"credential,omitempty"`
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Host        string              `json:"host"`
	Port        int                 `json:"port"` // Zero preserves the SSH config port.
	User        string              `json:"user"`
	Auth        string              `json:"auth"`
	Identity    string              `json:"identity"`
	Jump        string              `json:"jump"`
	AutoConnect bool                `json:"autoConnect"`
	Reconnect   bool                `json:"reconnect"`
	Mappings    []Mapping           `json:"mappings"`
}

type NetworkOptions struct {
	ConnectTimeout      int `json:"connectTimeout"`
	ServerAliveInterval int `json:"serverAliveInterval"`
	ServerAliveCountMax int `json:"serverAliveCountMax"`
}

func DefaultNetwork() NetworkOptions {
	return NetworkOptions{ConnectTimeout: 15, ServerAliveInterval: 15, ServerAliveCountMax: 3}
}

// Older configurations omit network and retain the original connection behaviour.
func (t Tunnel) EffectiveNetwork() NetworkOptions {
	if t.Network == nil {
		return DefaultNetwork()
	}
	return *t.Network
}

type Settings struct {
	Theme      string `json:"theme"`
	AutoUpdate bool   `json:"autoUpdate"`
	Language   string `json:"language"`
	SSHPath    string `json:"sshPath"`
}
type Config struct {
	Version  int      `json:"version"`
	Settings Settings `json:"settings"`
	Tunnels  []Tunnel `json:"tunnels"`
}

func Default() Config {
	return Config{Version: 1, Settings: Settings{Theme: "light", Language: "system", AutoUpdate: true}, Tunnels: []Tunnel{}}
}

var jumpPattern = regexp.MustCompile(`^(?:[a-zA-Z0-9_.-]+@)?(?:[a-zA-Z0-9_][a-zA-Z0-9_.-]*|\[[0-9a-fA-F:]+\])(?::[0-9]{1,5})?$`)

func clean(s string) bool { return !strings.ContainsAny(s, "\x00\r\n") }
func host(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && clean(s) && !strings.ContainsAny(s, " \t/@")
}
func ValidateTunnel(t Tunnel) error {
	n := t.EffectiveNetwork()
	if n.ConnectTimeout < 5 || n.ConnectTimeout > 120 || n.ServerAliveInterval < 5 || n.ServerAliveInterval > 300 || n.ServerAliveCountMax < 1 || n.ServerAliveCountMax > 10 {
		return errors.New("INVALID_NETWORK_OPTIONS")
	}
	if e := credentials.Validate(t.Credential); e != nil {
		return e
	}
	if t.Credential != nil && t.Auth != "password" {
		return errors.New("INVALID_CREDENTIAL")
	}
	if t.ID == "" || len(t.ID) > 128 || !clean(t.ID) || strings.TrimSpace(t.Name) == "" || len(t.Name) > 200 {
		return errors.New("INVALID_NAME")
	}
	if !host(t.Host) || (strings.Contains(t.Host, ":") && net.ParseIP(t.Host) == nil) {
		return errors.New("INVALID_HOST")
	}
	if t.Port < 0 || t.Port > 65535 || !clean(t.User) || strings.ContainsAny(t.User, " \t@") || strings.HasPrefix(t.User, "-") {
		return errors.New("INVALID_CONNECTION")
	}
	if t.Auth != "agent" && t.Auth != "key" && t.Auth != "password" {
		return errors.New("INVALID_AUTH")
	}
	if t.Auth == "key" && strings.TrimSpace(t.Identity) == "" {
		return errors.New("IDENTITY_REQUIRED")
	}
	if !clean(t.Identity) {
		return errors.New("INVALID_CONNECTION")
	}
	if t.Jump != "" {
		for _, j := range strings.Split(t.Jump, ",") {
			if !jumpPattern.MatchString(j) {
				return errors.New("INVALID_JUMP")
			}
		}
	}
	if len(t.Mappings) == 0 || len(t.Mappings) > 64 {
		return errors.New("INVALID_MAPPING")
	}
	for i, m := range t.Mappings {
		if m.Type != "local" && m.Type != "remote" && m.Type != "dynamic" {
			return errors.New("INVALID_MAPPING")
		}
		if net.ParseIP(m.Bind) == nil || m.Port < 1 || m.Port > 65535 {
			return errors.New("INVALID_MAPPING")
		}
		if m.Type != "dynamic" && (!host(m.Target) || strings.Contains(m.Target, ":") && net.ParseIP(m.Target) == nil || m.TargetPort < 1 || m.TargetPort > 65535) {
			return errors.New("INVALID_MAPPING")
		}

		for _, previous := range t.Mappings[:i] {
			sameSide := (m.Type == "remote") == (previous.Type == "remote")
			bind, prior := net.ParseIP(m.Bind), net.ParseIP(previous.Bind)
			sameFamily := (bind.To4() != nil) == (prior.To4() != nil)
			sameAddress := bind.Equal(prior) || sameFamily && (bind.IsUnspecified() || prior.IsUnspecified())
			if sameSide && sameAddress && m.Port == previous.Port {
				return errors.New("DUPLICATE_MAPPING")
			}
		}
	}
	return nil
}
func Validate(c Config) error {
	if c.Settings.Theme != "light" && c.Settings.Theme != "dark" {
		return errors.New("INVALID_THEME")
	}
	if c.Version != 1 {
		return errors.New("UNSUPPORTED_CONFIG_VERSION")
	}
	if c.Settings.Language != "system" && c.Settings.Language != "en" && c.Settings.Language != "zh-CN" {
		return errors.New("INVALID_LANGUAGE")
	}
	if !clean(c.Settings.SSHPath) {
		return errors.New("INVALID_SSH_PATH")
	}
	ids := map[string]bool{}
	for _, t := range c.Tunnels {
		if ids[t.ID] {
			return errors.New("DUPLICATE_ID")
		}
		ids[t.ID] = true
		if err := ValidateTunnel(t); err != nil {
			return err
		}
	}
	return nil
}
func Decode(data []byte) (Config, error) {
	c := Default()
	if strings.TrimSpace(string(data)) == "null" {
		return c, errors.New("INVALID_CONFIG")
	}
	// Accept the retired group field only when reading old configuration files.
	// It is discarded here and never enters application state or future exports.
	var wire struct {
		Config
		Tunnels []struct {
			Tunnel
			LegacyGroup string `json:"group"`
		} `json:"tunnels"`
	}
	wire.Config = c
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&wire); err != nil {
		return c, fmt.Errorf("INVALID_CONFIG: %w", err)
	}
	// Reject trailing JSON, including an accidentally concatenated second configuration.
	var extra any
	if err := d.Decode(&extra); err != nil && !errors.Is(err, io.EOF) {
		return c, errors.New("INVALID_CONFIG")
	} else if err == nil {
		return c, errors.New("INVALID_CONFIG")
	}
	c = wire.Config
	for _, entry := range wire.Tunnels {
		c.Tunnels = append(c.Tunnels, entry.Tunnel)
	}
	return c, Validate(c)
}

type Store struct {
	mu   sync.Mutex
	path string
}

func (s *Store) Path() string     { return s.path }
func NewStore(path string) *Store { return &Store{path: path} }
func DefaultPath() (string, error) {
	d, e := os.UserConfigDir()
	return filepath.Join(d, "Tunnelix", "config.json"), e
}

func selectedPathFile() (string, error) {
	d, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "Tunnelix", ".config-location"), nil
}

// RememberedPath returns the JSON file selected during first-run setup.
// The pointer contains no tunnel data or credentials.
func RememberedPath() (string, error) {
	path, e := selectedPathFile()
	if e != nil {
		return "", e
	}
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	value := strings.TrimSpace(string(b))
	if value == "" {
		return "", nil
	}
	return filepath.Abs(value)
}

// RememberPath persists only the selected configuration file location.
func RememberPath(path string) error {
	absolute, e := filepath.Abs(path)
	if e != nil || strings.TrimSpace(absolute) == "" {
		return errors.New("INVALID_CONFIG_PATH")
	}
	location, e := selectedPathFile()
	if e != nil {
		return e
	}
	return writeFile(location, []byte(absolute+"\n"))
}
func (s *Store) Load() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := os.ReadFile(s.path)
	if errors.Is(e, os.ErrNotExist) {
		return Default(), nil
	}
	if e != nil {
		return Config{}, e
	}
	return Decode(b)
}
func (s *Store) Save(c Config) error {
	if e := Validate(c); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return writeFile(s.path, append(b, '\n'))
}

// Create refuses to replace an existing file, including an existing symlink.
func (s *Store) Create(c Config) (result error) {
	if e := Validate(c); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(s.path), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer func() {
		if result != nil {
			os.Remove(s.path)
		}
	}()
	if _, e = f.Write(append(b, '\n')); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	return f.Close()
}

func writeFile(path string, data []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".config-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return replace(f.Name(), path)
}
