package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestNetworkDefaultsAndPersistence(t *testing.T) {
	c := Default()
	c.Tunnels = []Tunnel{sample()}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	old, err := Decode(data)
	if err != nil || old.Tunnels[0].EffectiveNetwork() != (NetworkOptions{15, 15, 3}) {
		t.Fatalf("legacy defaults: %+v %v", old, err)
	}
	want := NetworkOptions{45, 60, 5}
	c.Tunnels[0].Network = &want
	store := NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err = store.Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || got.Tunnels[0].EffectiveNetwork() != want {
		t.Fatalf("persist: %+v %v", got, err)
	}
}
func TestNetworkValidation(t *testing.T) {
	for _, n := range []NetworkOptions{{5, 5, 1}, {120, 300, 10}, {15, 15, 3}} {
		c := sample()
		c.Network = &n
		if err := ValidateTunnel(c); err != nil {
			t.Fatalf("valid %+v: %v", n, err)
		}
	}
	for _, n := range []NetworkOptions{{0, 15, 3}, {-1, 15, 3}, {4, 15, 3}, {121, 15, 3}, {15, 0, 3}, {15, 4, 3}, {15, 301, 3}, {15, 15, 0}, {15, 15, 11}} {
		c := sample()
		c.Network = &n
		if err := ValidateTunnel(c); err == nil || err.Error() != "INVALID_NETWORK_OPTIONS" {
			t.Fatalf("accepted invalid %+v: %v", n, err)
		}
	}
	for _, network := range []string{`{"connectTimeout":15.5,"serverAliveInterval":15,"serverAliveCountMax":3}`, `{"connectTimeout":15}`, `{"connectTimeout":15,"serverAliveInterval":15,"serverAliveCountMax":3,"extra":1}`} {
		c := Default()
		c.Tunnels = []Tunnel{sample()}
		data, _ := json.Marshal(c)
		var doc map[string]any
		json.Unmarshal(data, &doc)
		var n any
		json.Unmarshal([]byte(network), &n)
		doc["tunnels"].([]any)[0].(map[string]any)["network"] = n
		data, _ = json.Marshal(doc)
		if _, err := Decode(data); err == nil {
			t.Fatalf("accepted malformed network: %s", network)
		}
	}
}
