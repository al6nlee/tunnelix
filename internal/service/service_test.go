package service

import (
	"encoding/json"
	"errors"
	"github.com/zalando/go-keyring"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tunnelix/internal/config"
	"tunnelix/internal/credentials"
	"tunnelix/internal/tunnel"
)

func newService(t *testing.T) *Service {
	t.Helper()
	helper, _ := os.Executable()
	m := tunnel.New(helper)
	t.Cleanup(m.Close)
	s, e := New(config.NewStore(filepath.Join(t.TempDir(), "config.json")), m)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func testTunnel() config.Tunnel {
	return config.Tunnel{ID: "test", Name: "Database", Host: "localhost", Auth: "password", AutoConnect: true, Mappings: []config.Mapping{{Type: "local", Bind: "127.0.0.1", Port: 5432, Target: "localhost", TargetPort: 5432}}}
}
func TestConfigImportIsAtomicAndDisablesAutoconnect(t *testing.T) {
	s := newService(t)
	c := config.Default()
	c.Tunnels = []config.Tunnel{testTunnel()}
	b, _ := json.Marshal(c)
	// Previously exported files may still contain a group; import it without
	// reintroducing the retired field into snapshots or exports.
	b = []byte(strings.Replace(string(b), `"id":"test"`, `"group":"Work","id":"test"`, 1))
	if e := s.ImportConfig(string(b)); e != nil {
		t.Fatal(e)
	}
	got := s.Snapshot()
	if got.Config.Tunnels[0].AutoConnect {
		t.Fatal("import auto-connected")
	}
	if e := s.ImportConfig(string(b)); e == nil || e.Error() != "DUPLICATE_ID" {
		t.Fatal("duplicate not rejected", e)
	}
	if len(s.Snapshot().Config.Tunnels) != 1 {
		t.Fatal("import partially changed state")
	}
	export, e := s.ExportConfig()
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(export, `"password":`) {
		t.Fatal("secret field exported")
	}
	snapshot, _ := json.Marshal(got)
	if strings.Contains(export, `"group"`) || strings.Contains(string(snapshot), `"group"`) {
		t.Fatal("legacy group escaped into application state")
	}
	got.Config.Tunnels[0].Mappings[0].Port = 1
	if s.Snapshot().Config.Tunnels[0].Mappings[0].Port != 5432 {
		t.Fatal("snapshot aliases mutable configuration")
	}
}
func TestSaveDeleteAndSettingsPersist(t *testing.T) {
	s := newService(t)
	if e := s.SaveTunnel(testTunnel()); e != nil {
		t.Fatal(e)
	}
	settings := config.Settings{Theme: "dark", Language: "zh-CN", SSHPath: "/custom/ssh"}
	if e := s.SaveSettings(settings); e != nil {
		t.Fatal(e)
	}
	got, e := s.store.Load()
	if e != nil || got.Settings != settings || len(got.Tunnels) != 1 {
		t.Fatal(got, e)
	}
	if e = s.DeleteTunnel("test"); e != nil {
		t.Fatal(e)
	}
	got, e = s.store.Load()
	if e != nil || len(got.Tunnels) != 0 {
		t.Fatal(got, e)
	}
}

func TestSelectConfigPathCreatesAndUsesJSONFile(t *testing.T) {
	s := newService(t)
	path := filepath.Join(t.TempDir(), "chosen", "config.json")
	if e := s.SelectConfigPath(path); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); e != nil {
		t.Fatal("selected configuration was not created", e)
	}
	settings := config.Default().Settings
	settings.Theme = "dark"
	if e := s.SaveSettings(settings); e != nil {
		t.Fatal(e)
	}
	loaded, e := config.NewStore(path).Load()
	if e != nil || loaded.Settings.Theme != "dark" {
		t.Fatalf("selected configuration was not used: %+v %v", loaded.Settings, e)
	}
}

func TestPreviewUsesLaunchArgumentsWithoutCredentials(t *testing.T) {
	s := newService(t)
	c := testTunnel()
	command, e := s.PreviewCommand(c)
	if e != nil {
		t.Fatal(e)
	}
	for _, word := range []string{"-L", "127.0.0.1:5432:localhost:5432", "PreferredAuthentications=password"} {
		if !strings.Contains(command, word) {
			t.Fatalf("missing %s: %s", word, command)
		}
	}
	if strings.Contains(command, "TUNNELIX_ASKPASS") || strings.Contains(command, "SSH_ASKPASS=") {
		t.Fatal("IPC information in preview")
	}
}
func TestInvalidDraftIsNeverPersisted(t *testing.T) {
	s := newService(t)
	draft := testTunnel()
	draft.Host = "-bad-host"
	if _, e := s.StartTest(draft); e == nil {
		t.Fatal("invalid draft accepted")
	}
	snapshot := s.Snapshot()
	if len(snapshot.Config.Tunnels) != 0 || len(snapshot.Statuses) != 0 || len(snapshot.Tests) != 0 {
		t.Fatal("test changed application configuration", snapshot)
	}
	c, e := s.store.Load()
	if e != nil || len(c.Tunnels) != 0 {
		t.Fatal(c, e)
	}
}

type testKeys struct {
	values map[string]string
	fail   bool
}

func (k *testKeys) Get(s, u string) (string, error) {
	if k.fail {
		return "", errors.New("unavailable")
	}
	v, ok := k.values[s+u]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}
func (k *testKeys) Set(s, u, v string) error {
	if k.fail {
		return errors.New("unavailable")
	}
	k.values[s+u] = v
	return nil
}
func TestCredentialModesRedactionAndLifecycle(t *testing.T) {
	s := newService(t)
	keys := &testKeys{values: map[string]string{}}
	s.vault = credentials.NewWithKeyring("service-test", keys)
	connection := testTunnel()
	for _, mode := range []string{"plain", "encrypted", "plain"} {
		value := ""
		if mode == "plain" && len(s.cfg.Tunnels) == 0 {
			value = "private-password"
		}
		if e := s.SaveTunnelWithCredential(connection, credentials.Update{Mode: mode, Value: value}); e != nil {
			t.Fatal(e)
		}
		stored, e := s.store.Load()
		if e != nil {
			t.Fatal(e)
		}
		got, e := s.vault.Open(stored.Tunnels[0].Credential, credentialScope(connection))
		if e != nil || got != "private-password" {
			t.Fatal("lost credential", e)
		}
		snapshot, _ := json.Marshal(s.Snapshot())
		export, e := s.ExportConfig()
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(snapshot), "private-password") || strings.Contains(export, "private-password") || strings.Contains(export, `"credential"`) {
			t.Fatal("credential escaped redaction")
		}
		if s.Snapshot().Credentials[connection.ID].Mode != mode {
			t.Fatal("metadata missing")
		}
		raw, _ := os.ReadFile(s.store.Path())
		if mode == "encrypted" && strings.Contains(string(raw), "private-password") {
			t.Fatal("encrypted config leaked secret")
		}
	}
	connection.Name = "Renamed"
	if e := s.SaveTunnel(connection); e != nil {
		t.Fatal(e)
	}
	if !s.Snapshot().Credentials[connection.ID].Saved {
		t.Fatal("rename lost password")
	}
	connection.User = "different-user"
	if e := s.SaveTunnel(connection); e != nil {
		t.Fatal(e)
	}
	if s.Snapshot().Credentials[connection.ID].Saved {
		t.Fatal("identity change kept password")
	}
	if e := s.SaveTunnelWithCredential(connection, credentials.Update{Mode: "encrypted", Value: "replacement"}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveTunnelWithCredential(connection, credentials.Update{Mode: "none"}); e != nil {
		t.Fatal(e)
	}
	if s.Snapshot().Credentials[connection.ID].Saved {
		t.Fatal("remove kept password")
	}
	if e := s.SaveTunnelWithCredential(connection, credentials.Update{Mode: "plain", Value: "delete-me"}); e != nil {
		t.Fatal(e)
	}
	if e := s.DeleteTunnel(connection.ID); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(s.store.Path())
	if strings.Contains(string(raw), "delete-me") {
		t.Fatal("deleted credential remains")
	}
}
func TestCredentialFailureLeavesConfigUnchangedAndImportRejectsSecrets(t *testing.T) {
	s := newService(t)
	s.vault = credentials.NewWithKeyring("failure", &testKeys{fail: true})
	connection := testTunnel()
	if e := s.SaveTunnel(connection); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(s.store.Path())
	connection.Name = "Must not save"
	if e := s.SaveTunnelWithCredential(connection, credentials.Update{Mode: "encrypted", Value: "secret"}); e == nil {
		t.Fatal("unavailable keyring accepted")
	}
	after, _ := os.ReadFile(s.store.Path())
	if string(before) != string(after) {
		t.Fatal("partial config update")
	}
	c := config.Default()
	connection.ID = "incoming"
	connection.Credential = &credentials.Record{Mode: "plain", Value: "must-not-import"}
	c.Tunnels = []config.Tunnel{connection}
	data, _ := json.Marshal(c)
	if e := s.ImportConfig(string(data)); e == nil || e.Error() != "IMPORT_CONTAINS_CREDENTIALS" {
		t.Fatal(e)
	}
	if len(s.Snapshot().Config.Tunnels) != 1 {
		t.Fatal("partial import")
	}
}

func TestAutoConnectReportsLockedCredentialStore(t *testing.T) {
	s := newService(t)
	keys := &testKeys{values: map[string]string{}}
	s.vault = credentials.NewWithKeyring("autoconnect", keys)
	c := testTunnel()
	if e := s.SaveTunnelWithCredential(c, credentials.Update{Mode: "encrypted", Value: "secret"}); e != nil {
		t.Fatal(e)
	}
	keys.fail = true
	s.AutoConnect()
	snapshot := s.Snapshot()
	if len(snapshot.Statuses) != 1 || snapshot.Statuses[0].Code != "CREDENTIAL_STORE_UNAVAILABLE" || snapshot.Statuses[0].State != "error" {
		t.Fatal("auto-connect failure hidden", snapshot.Statuses)
	}
	if s.manager.Active(c.ID) {
		t.Fatal("SSH started despite unavailable credentials")
	}
}

func TestNetworkOptionsSaveExportImportAndPreview(t *testing.T) {
	s := newService(t)
	c := testTunnel()
	want := config.NetworkOptions{ConnectTimeout: 45, ServerAliveInterval: 60, ServerAliveCountMax: 5}
	c.Network = &want
	if err := s.SaveTunnel(c); err != nil {
		t.Fatal(err)
	}
	exported, err := s.ExportConfig()
	if err != nil {
		t.Fatal(err)
	}
	other := newService(t)
	if err = other.ImportConfig(exported); err != nil {
		t.Fatal(err)
	}
	loaded, err := other.store.Load()
	if err != nil || loaded.Tunnels[0].EffectiveNetwork() != want {
		t.Fatalf("import lost network settings: %+v %v", loaded, err)
	}
	preview, err := other.PreviewCommand(loaded.Tunnels[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"ConnectTimeout=45", "ServerAliveInterval=60", "ServerAliveCountMax=5"} {
		if !strings.Contains(preview, option) {
			t.Fatalf("preview missing %s: %s", option, preview)
		}
	}
	invalid := config.NetworkOptions{ConnectTimeout: 0, ServerAliveInterval: 60, ServerAliveCountMax: 5}
	c.Network = &invalid
	if err = s.SaveTunnel(c); err == nil {
		t.Fatal("invalid settings saved")
	}
	if _, err = s.StartTest(c); err == nil {
		t.Fatal("invalid draft started")
	}
	if s.Snapshot().Config.Tunnels[0].EffectiveNetwork() != want {
		t.Fatal("invalid draft changed configuration")
	}
}
