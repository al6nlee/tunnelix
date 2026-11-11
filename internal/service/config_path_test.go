package service

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"tunnelix/internal/config"
	"tunnelix/internal/credentials"
	"tunnelix/internal/tunnel"
)

func TestRelocateConfigPreservesCredentialsAndWritesOnlyToNewFile(t *testing.T) {
	for _, mode := range []string{"plain", "encrypted"} {
		t.Run(mode, func(t *testing.T) {
			s := newService(t)
			keys := &testKeys{values: map[string]string{}}
			s.vault = credentials.NewWithKeyring(s.store.Path(), keys)
			connection := testTunnel()
			if err := s.SaveTunnelWithCredential(connection, credentials.Update{Mode: mode, Value: "saved-password"}); err != nil {
				t.Fatal(err)
			}
			original := s.store.Path()
			before, err := os.ReadFile(original)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "nested", "relocated.json")
			remembered := ""
			s.rememberPath = func(path string) error { remembered = path; return nil }
			if err = s.ChangeConfigPath(target, true); err != nil {
				t.Fatal(err)
			}
			if s.Snapshot().ConfigPath != target || remembered != target {
				t.Fatal("new configuration path was not activated and remembered")
			}
			loaded, err := config.NewStore(target).Load()
			if err != nil || len(loaded.Tunnels) != 1 {
				t.Fatal("configuration lost during relocation", err)
			}
			// Reopening under a new path uses each record's stored key ID. The
			// same keyring backend represents the unchanged system account.
			vault := credentials.NewWithKeyring(target, keys)
			if secret, err := vault.Open(loaded.Tunnels[0].Credential, credentialScope(connection)); err != nil || secret != "saved-password" {
				t.Fatal("saved password cannot be reopened after relocation", err)
			}
			connection.Name = "Changed after relocation"
			if err = s.SaveTunnel(connection); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(original)
			if err != nil || string(after) != string(before) {
				t.Fatal("original file was modified", err)
			}
			loaded, err = config.NewStore(target).Load()
			if err != nil || loaded.Tunnels[0].Name != connection.Name || loaded.Tunnels[0].Credential == nil {
				t.Fatal("changes did not persist to the new file", err)
			}
			restarted, err := New(config.NewStore(remembered), s.manager)
			if err != nil || restarted.Snapshot().ConfigPath != target || restarted.Snapshot().Config.Tunnels[0].Name != connection.Name {
				t.Fatal("new configuration could not be reopened", err)
			}
		})
	}
}

func TestSwitchConfigLoadsSettingsAndClearsOldConnectionHistory(t *testing.T) {
	s := newService(t)
	if err := s.SaveTunnel(testTunnel()); err != nil {
		t.Fatal(err)
	}
	s.manager.RecordError("test", "AUTH_FAILED")
	original := s.store.Path()
	c := config.Default()
	c.Settings.Theme = "dark"
	c.Settings.Language = "en"
	connection := testTunnel()
	connection.Name = "From another file"
	connection.Credential = &credentials.Record{Mode: "plain", Value: "existing-password"}
	c.Tunnels = []config.Tunnel{connection}
	target := filepath.Join(t.TempDir(), "existing.json")
	if err := config.NewStore(target).Save(c); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	remembered := ""
	s.rememberPath = func(path string) error { remembered = path; return nil }
	if err = s.ChangeConfigPath(target, false); err != nil {
		t.Fatal(err)
	}
	got := s.Snapshot()
	if got.ConfigPath != target || remembered != target || got.Config.Settings != c.Settings || got.Config.Tunnels[0].Name != connection.Name {
		t.Fatal("existing file was not fully loaded")
	}
	if len(got.Statuses) != 0 || len(got.Tests) != 0 || s.manager.HasActiveSessions() {
		t.Fatal("old history remained or switch started auto-connect")
	}
	if secret, err := s.vault.Open(s.cfg.Tunnels[0].Credential, credentialScope(connection)); err != nil || secret != "existing-password" {
		t.Fatal("existing password was lost", err)
	}
	after, err := os.ReadFile(target)
	if err != nil || string(before) != string(after) {
		t.Fatal("switch rewrote the selected file", err)
	}
	settings := got.Config.Settings
	settings.Language = "zh-CN"
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.NewStore(original).Load()
	if err != nil || loaded.Tunnels[0].Name != "Database" {
		t.Fatal("switch modified the previous file", err)
	}
	loaded, err = config.NewStore(target).Load()
	if err != nil || loaded.Settings.Language != "zh-CN" {
		t.Fatal("settings were not saved to the selected file", err)
	}
}

func TestConfigPathFailureKeepsOriginalConfiguration(t *testing.T) {
	for _, scenario := range []string{"exists", "invalid", "missing", "remember-move", "remember-switch", "unwritable"} {
		t.Run(scenario, func(t *testing.T) {
			s := newService(t)
			if err := s.SaveTunnel(testTunnel()); err != nil {
				t.Fatal(err)
			}
			before := s.Snapshot()
			target := filepath.Join(t.TempDir(), "target.json")
			migrate := true
			want := "CONFIG_PATH_EXISTS"
			switch scenario {
			case "exists":
				if err := os.WriteFile(target, []byte("do not overwrite"), 0600); err != nil {
					t.Fatal(err)
				}
			case "invalid":
				migrate, want = false, "INVALID_CONFIG"
				if err := os.WriteFile(target, []byte("broken json"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				migrate, want = false, "CONFIG_PATH_READ_FAILED"
			case "remember-move", "remember-switch":
				want = "CONFIG_PATH_REMEMBER_FAILED"
				s.rememberPath = func(string) error { return errors.New("permission denied") }
				if scenario == "remember-switch" {
					migrate = false
					if err := config.NewStore(target).Save(config.Default()); err != nil {
						t.Fatal(err)
					}
				}
			case "unwritable":
				want = "CONFIG_PATH_WRITE_FAILED"
				if err := os.WriteFile(target, []byte("parent is a file"), 0600); err != nil {
					t.Fatal(err)
				}
				target = filepath.Join(target, "config.json")
			}
			if err := s.ChangeConfigPath(target, migrate); err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("got %v, want %s", err, want)
			}
			if !reflect.DeepEqual(s.Snapshot(), before) {
				t.Fatal("failed operation changed the active configuration")
			}
			if scenario == "exists" {
				data, err := os.ReadFile(target)
				if err != nil || string(data) != "do not overwrite" {
					t.Fatal("existing destination was overwritten", err)
				}
			}
			if scenario == "remember-move" {
				if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed migration left a new file behind", err)
				}
			}
		})
	}
}

func TestConfigPathChangeRejectsActiveConnectionsAndTests(t *testing.T) {
	helper := os.Getenv("TUNNELIX_TEST_HELPER")
	if helper == "" {
		t.Skip("requires compiled desktop helper via TUNNELIX_TEST_HELPER")
	}
	for _, isTest := range []bool{false, true} {
		t.Run(strconv.FormatBool(isTest), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				conn, err := listener.Accept()
				if err == nil {
					accepted <- conn
				}
			}()
			manager := tunnel.New(helper)
			defer manager.Close()
			s, err := New(config.NewStore(filepath.Join(t.TempDir(), "original.json")), manager)
			if err != nil {
				t.Fatal(err)
			}
			s.rememberPath = func(string) error { t.Error("active configuration was remembered"); return nil }
			connection := testTunnel()
			connection.Host = "127.0.0.1"
			connection.Port = listener.Addr().(*net.TCPAddr).Port
			connection.AutoConnect = false
			if isTest {
				_, err = s.StartTest(connection)
			} else {
				if err = s.SaveTunnel(connection); err == nil {
					err = s.Connect(connection.ID)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case conn := <-accepted:
				defer conn.Close()
			case <-time.After(10 * time.Second):
				t.Fatal("SSH helper did not reach the local listener")
			}
			target := filepath.Join(t.TempDir(), "new.json")
			for _, migrate := range []bool{true, false} {
				if err = s.ChangeConfigPath(target, migrate); err == nil || err.Error() != "CONFIG_ACTIVE_CONNECTIONS" {
					t.Fatal("configuration changed while a session was running", err)
				}
			}
			if _, err = os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("blocked migration created a file", err)
			}
		})
	}
}
