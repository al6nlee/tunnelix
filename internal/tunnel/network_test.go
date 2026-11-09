package tunnel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"tunnelix/internal/config"
)

func TestOpenSSHResolvesNetworkOptions(t *testing.T) {
	ssh, err := Executable("")
	if err != nil {
		t.Fatal(err)
	}
	// A dedicated empty config prevents reading user SSH aliases or running local commands.
	path := filepath.Join(t.TempDir(), "ssh_config")
	if err = os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		network *config.NetworkOptions
		want    config.NetworkOptions
	}{
		{"legacy", nil, config.NetworkOptions{ConnectTimeout: 15, ServerAliveInterval: 15, ServerAliveCountMax: 3}},
		{"custom", &config.NetworkOptions{ConnectTimeout: 45, ServerAliveInterval: 60, ServerAliveCountMax: 5}, config.NetworkOptions{ConnectTimeout: 45, ServerAliveInterval: 60, ServerAliveCountMax: 5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Tunnel{Host: "qa.invalid", Auth: "agent", Network: tc.network}
			args := append([]string{"-F", path, "-G"}, Arguments(cfg)...)
			out, err := exec.Command(ssh, args...).Output()
			if err != nil {
				t.Fatal(err)
			}
			resolved := map[string]string{}
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					resolved[fields[0]] = fields[1]
				}
			}
			for key, want := range map[string]int{"connecttimeout": tc.want.ConnectTimeout, "serveraliveinterval": tc.want.ServerAliveInterval, "serveralivecountmax": tc.want.ServerAliveCountMax} {
				if resolved[key] != strconv.Itoa(want) {
					t.Fatalf("%s = %q, want %d", key, resolved[key], want)
				}
			}
		})
	}
}
