package tray

import (
	"fmt"
	"sort"
	"strings"
	"tunnelix/internal/config"
	"tunnelix/internal/tunnel"
)

type Entry struct {
	ID, Name, State, Action string
	Active                  bool
}

const EntryLimit = 10

type Model struct {
	Entries                             []Entry
	Connected, Active, Total, Attention int
	Summary                             string
	ViewAll                             string
}

func Build(cfg config.Config, statuses []tunnel.Status, zh bool) Model {
	label := func(en, cn string) string {
		if zh {
			return cn
		}
		return en
	}
	states := map[string]string{}
	for _, s := range statuses {
		if !s.IsTest {
			states[s.ID] = s.State
		}
	}
	m := Model{Total: len(cfg.Tunnels)}
	for _, t := range cfg.Tunnels {
		state := states[t.ID]
		active := state == "connected" || state == "connecting" || state == "reconnecting"
		if active {
			m.Active++
		}
		if state == "connected" {
			m.Connected++
		}
		if state == "error" {
			m.Attention++
		}
		status := label("Disconnected", "未连接")
		switch state {
		case "connected":
			status = label("Connected", "已连接")
		case "connecting":
			status = label("Connecting…", "连接中…")
		case "reconnecting":
			status = label("Reconnecting…", "重连中…")
		case "error":
			status = label("Needs attention", "连接异常")
		}
		action := label("Connect", "连接")
		if active {
			action = label("Disconnect", "断开")
		}
		m.Entries = append(m.Entries, Entry{ID: t.ID, Name: clean(t.Name), State: status, Action: action, Active: active})
	}
	sort.SliceStable(m.Entries, func(i, j int) bool {
		if m.Entries[i].Active != m.Entries[j].Active {
			return m.Entries[i].Active
		}
		return strings.ToLower(m.Entries[i].Name) < strings.ToLower(m.Entries[j].Name)
	})
	if len(m.Entries) > EntryLimit {
		m.Entries = m.Entries[:EntryLimit]
		m.ViewAll = fmt.Sprintf(label("View all %d connections…", "查看全部 %d 条连接…"), m.Total)
	}
	m.Summary = fmt.Sprintf(label("%d of %d connected", "%d / %d 条已连接"), m.Connected, m.Total)
	if m.Attention > 0 {
		m.Summary += fmt.Sprintf(label(" · %d need attention", " · %d 条异常"), m.Attention)
	}
	return m
}

// Native menu labels must not accept newlines, accelerator tabs, or mnemonics.
func clean(s string) string {
	r := []rune(strings.Map(func(r rune) rune {
		if r < ' ' || r == 127 {
			return ' '
		}
		if r == '&' {
			return '＆'
		}
		return r
	}, s))
	if len(r) > 36 {
		return string(r[:35]) + "…"
	}
	return string(r)
}
