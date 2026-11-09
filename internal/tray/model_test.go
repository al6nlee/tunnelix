package tray

import (
	"fmt"
	"strings"
	"testing"
	"tunnelix/internal/config"
	"tunnelix/internal/tunnel"
)

func TestTrayLimitKeepsFullCountsAndActivePriority(t *testing.T) {
	for _, count := range []int{0, 10, 11, 35} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			cfg := config.Default()
			for i := 0; i < count; i++ {
				cfg.Tunnels = append(cfg.Tunnels, config.Tunnel{ID: fmt.Sprint(i), Name: fmt.Sprintf("Tunnel %02d", i)})
			}
			states := []tunnel.Status{{ID: fmt.Sprint(count - 1), State: "connected"}, {ID: "0", State: "error"}}
			m := Build(cfg, states, true)
			if len(m.Entries) != min(count, EntryLimit) || m.Total != count {
				t.Fatalf("incorrect visible/total counts: %+v", m)
			}
			if count > 0 && (m.Entries[0].ID != fmt.Sprint(count-1) || m.Active != 1 || m.Connected != 1 || m.Attention != 1) {
				t.Fatalf("lost active priority or full counts: %+v", m)
			}
			if count > EntryLimit {
				if m.ViewAll != fmt.Sprintf("查看全部 %d 条连接…", count) || Build(cfg, states, false).ViewAll != fmt.Sprintf("View all %d connections…", count) {
					t.Fatal("incorrect view-all labels")
				}
			} else if m.ViewAll != "" {
				t.Fatal("unnecessary view-all entry")
			}
		})
	}
	// Even when every connection is active, the menu stays bounded and the
	// batch-action counts still include connections hidden from the tray.
	cfg := config.Default()
	var states []tunnel.Status
	for i := 0; i < 15; i++ {
		id := fmt.Sprint(i)
		cfg.Tunnels = append(cfg.Tunnels, config.Tunnel{ID: id, Name: id})
		states = append(states, tunnel.Status{ID: id, State: "connected"})
	}
	m := Build(cfg, states, false)
	if len(m.Entries) != EntryLimit || m.Active != 15 || m.Connected != 15 || m.Total != 15 {
		t.Fatal(m)
	}
}

func TestFlatTray(t *testing.T) {
	cfg := config.Default()
	cfg.Tunnels = []config.Tunnel{{ID: "1", Name: "alpha"}, {ID: "2", Name: "zeta"}, {ID: "3", Name: "Other"}, {ID: "4", Name: "failed"}}
	statuses := []tunnel.Status{{ID: "2", State: "connected"}, {ID: "4", State: "error"}, {ID: "1", State: "connected", IsTest: true}}
	m := Build(cfg, statuses, true)
	if m.Total != 4 || m.Active != 1 || m.Attention != 1 || m.Connected != 1 {
		t.Fatalf("counts %+v", m)
	}
	if len(m.Entries) != 4 {
		t.Fatalf("entries %+v", m.Entries)
	}
	for i, id := range []string{"2", "1", "4", "3"} {
		if m.Entries[i].ID != id {
			t.Fatalf("entry %d: got %q, want %q", i, m.Entries[i].ID, id)
		}
	}
	if m.Entries[0].Action != "断开" || m.Entries[1].Action != "连接" {
		t.Fatal("actions")
	}
	if !strings.Contains(Build(cfg, statuses, false).Summary, "need attention") {
		t.Fatal("English summary")
	}
}
func TestMenuLabels(t *testing.T) {
	if got := clean("A&B\tC\nD"); got != "A＆B C D" {
		t.Fatal(got)
	}
	if len([]rune(clean(strings.Repeat("隧", 50)))) != 36 {
		t.Fatal("truncation")
	}
	if m := Build(config.Default(), nil, false); m.Summary != "0 of 0 connected" || len(m.Entries) != 0 {
		t.Fatal(m)
	}
}
