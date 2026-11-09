package tunnel

import (
	"testing"
	"tunnelix/internal/config"
)

func TestUpdateRestartReservation(t *testing.T) {
	m := New("")
	defer m.Close()
	m.sessions["active"] = &session{}
	if e := m.ReserveRestart(); e == nil {
		t.Fatal("restart accepted active session")
	}
	delete(m.sessions, "active")
	if e := m.ReserveRestart(); e != nil {
		t.Fatal(e)
	}
	// Validation and executable lookup still run, but no process may start.
	cfg := config.Tunnel{ID: "test", Name: "test", Host: "localhost", Auth: "agent", Mappings: []config.Mapping{{Type: "dynamic", Bind: "127.0.0.1", Port: 25432}}}
	if e := m.Connect(cfg, ""); e == nil || e.Error() != "APP_CLOSED" {
		t.Fatalf("connection during restart: %v", e)
	}
	if _, e := m.StartTest(cfg, ""); e == nil || e.Error() != "APP_CLOSED" {
		t.Fatalf("test during restart: %v", e)
	}
	m.ReleaseRestart()
	if e := m.ReserveRestart(); e != nil {
		t.Fatal("reservation not released")
	}
}
