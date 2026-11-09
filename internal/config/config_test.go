package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"tunnelix/internal/credentials"
)

func sample() Tunnel {
	return Tunnel{ID: "one", Name: "Database", Host: "example.com", Auth: "password", Mappings: []Mapping{{Type: "local", Bind: "127.0.0.1", Port: 5432, Target: "db.internal", TargetPort: 5432}}}
}
func TestStoreAndRejectSecrets(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	store := NewStore(p)
	c := Default()
	c.Tunnels = append(c.Tunnels, sample())
	if e := store.Save(c); e != nil {
		t.Fatal(e)
	}
	got, e := store.Load()
	if e != nil || len(got.Tunnels) != 1 {
		t.Fatalf("load: %v %v", got, e)
	}
	c.Tunnels[0].Name = "Updated"
	if e = store.Save(c); e != nil {
		t.Fatal(e)
	}
	got, e = store.Load()
	if e != nil || got.Tunnels[0].Name != "Updated" {
		t.Fatal("replacement failed", e)
	}
	b, _ := os.ReadFile(p)
	for _, bad := range []string{strings.Replace(string(b), `"auth": "password"`, `"auth": "password", "password": "secret"`, 1), string(b) + `{}`, `{"version":2}`} {
		if _, e = Decode([]byte(bad)); e == nil {
			t.Fatal("accepted invalid configuration")
		}
	}
}
func TestValidationBlocksOptionsAndInvalidMappings(t *testing.T) {
	for _, mutate := range []func(*Tunnel){func(t *Tunnel) { t.Host = "-oProxyCommand=bad" }, func(t *Tunnel) { t.Host = "foo\nbar" }, func(t *Tunnel) { t.Port = 65536 }, func(t *Tunnel) { t.Mappings[0].Port = 0 }, func(t *Tunnel) { t.Mappings[0].Bind = "garbage" }, func(t *Tunnel) { t.Mappings[0].Target = "host:99" }, func(t *Tunnel) { t.Jump = "-oBad" }} {
		v := sample()
		mutate(&v)
		if ValidateTunnel(v) == nil {
			t.Fatal("accepted invalid tunnel", v)
		}
	}
}
func TestCorruptConfigNotOverwritten(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte("broken"), 0600)
	if _, e := NewStore(p).Load(); e == nil {
		t.Fatal("corrupt config silently accepted")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "broken" {
		t.Fatal("changed corrupt config")
	}
}
func TestNullTunnelListNormalizesToEmpty(t *testing.T) {
	c, e := Decode([]byte(`{"version":1,"settings":{"language":"en"},"tunnels":null}`))
	if e != nil || c.Tunnels == nil {
		t.Fatal("null list not normalized", e)
	}
	if _, e = Decode([]byte(`null`)); e == nil {
		t.Fatal("null config accepted")
	}
}

func TestOverlappingMappingsAreRejected(t *testing.T) {
	c := sample()
	other := c.Mappings[0]
	other.Type = "dynamic"
	c.Mappings = append(c.Mappings, other)
	if e := ValidateTunnel(c); e == nil || e.Error() != "DUPLICATE_MAPPING" {
		t.Fatal(e)
	}
	c.Mappings[1].Type = "remote"
	if e := ValidateTunnel(c); e != nil {
		t.Fatal("local and remote are separate listeners", e)
	}
	c.Mappings[1].Type = "local"
	c.Mappings[1].Bind = "0.0.0.0"
	if e := ValidateTunnel(c); e == nil {
		t.Fatal("wildcard overlap accepted")
	}
	c.Mappings[1].Bind = "::"
	if e := ValidateTunnel(c); e != nil {
		t.Fatal("separate address families rejected", e)
	}
	c.Mappings[0].Bind = "::1"
	c.Mappings[1].Bind = "0:0:0:0:0:0:0:1"
	if e := ValidateTunnel(c); e == nil {
		t.Fatal("equivalent IPv6 listener overlap accepted")
	}
}

func TestAutoUpdateDefaultAndOptOut(t *testing.T) {
	c, err := Decode([]byte(`{"version":1,"settings":{"language":"en","sshPath":""},"tunnels":[]}`))
	if err != nil || !c.Settings.AutoUpdate {
		t.Fatalf("old config default: %v %+v", err, c.Settings)
	}
	c.Settings.AutoUpdate = false
	store := NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err = store.Save(c); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Settings.AutoUpdate {
		t.Fatalf("opt out not persisted: %v", err)
	}
}

func TestThemeMigrationAndPersistence(t *testing.T) {
	c, err := Decode([]byte(`{"version":1,"settings":{"language":"en"},"tunnels":[]}`))
	if err != nil || c.Settings.Theme != "light" {
		t.Fatalf("legacy config must default to light: %+v %v", c.Settings, err)
	}
	store := NewStore(filepath.Join(t.TempDir(), "config.json"))
	for _, theme := range []string{"dark", "light"} {
		c.Settings.Theme = theme
		if err := store.Save(c); err != nil {
			t.Fatal(err)
		}
		got, err := store.Load()
		if err != nil || got.Settings.Theme != theme {
			t.Fatalf("theme did not persist: %+v %v", got.Settings, err)
		}
	}
	c.Settings.Theme = "system"
	if err := store.Save(c); err == nil || err.Error() != "INVALID_THEME" {
		t.Fatalf("accepted unsupported theme: %v", err)
	}
	got, err := store.Load()
	if err != nil || got.Settings.Theme != "light" {
		t.Fatalf("invalid save changed config: %+v %v", got.Settings, err)
	}
}

func TestLegacyGroupDiscardedWithoutChangingConnection(t *testing.T) {
	c := Default()
	c.Settings.AutoUpdate = false
	first := sample()
	first.AutoConnect = true
	first.Network = &NetworkOptions{ConnectTimeout: 30, ServerAliveInterval: 60, ServerAliveCountMax: 5}
	first.Credential = &credentials.Record{Mode: "plain", Value: "test-password"}
	second := sample()
	second.ID = "two"
	c.Tunnels = []Tunnel{first, second}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(string(b), `"id":"one"`, `"group":"Production","id":"one"`, 1)
	legacy = strings.Replace(legacy, `"id":"two"`, `"group":"","id":"two"`, 1)
	path := filepath.Join(t.TempDir(), "config.json")
	if err = os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	got, err := store.Load()
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("legacy configuration changed: %v", err)
	}
	beforeSave, err := os.ReadFile(path)
	if err != nil || string(beforeSave) != legacy {
		t.Fatal("loading rewrote configuration", err)
	}
	if err = store.Save(got); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(saved), `"group"`) {
		t.Fatal("saved configuration still contains group", err)
	}
	reloaded, err := store.Load()
	if err != nil || !reflect.DeepEqual(reloaded, c) {
		t.Fatal("connection data lost on save", err)
	}
	for _, bad := range []string{
		strings.Replace(legacy, `"group":"Production"`, `"group":"Production","unknown":true`, 1),
		strings.Replace(legacy, `"type":"local"`, `"type":"local","unknown":true`, 1),
		strings.Replace(legacy, `"mode":"plain"`, `"mode":"plain","unknown":true`, 1),
	} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Fatal("legacy compatibility allowed unknown fields")
		}
	}
}
