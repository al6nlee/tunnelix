package tunnel

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"tunnelix/internal/askpass"
	"tunnelix/internal/config"
	"tunnelix/internal/processguard"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == processguard.Argument {
		os.Exit(processguard.Run(os.Args[2:]))
	}
	// This test binary also acts as a portable SSH shim, avoiding shell scripts.
	if len(os.Args) > 1 && os.Args[1] == "-v" {
		executable, _ := os.Executable()
		configPath := filepath.Join(filepath.Dir(executable), "ssh_config")
		sshPath, e := exec.LookPath("ssh")
		if e != nil {
			os.Exit(125)
		}
		cmd := exec.Command(sshPath, append([]string{"-F", configPath}, os.Args[1:]...)...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if e = cmd.Run(); e != nil {
			if ex, ok := e.(*exec.ExitError); ok {
				os.Exit(ex.ExitCode())
			}
			os.Exit(125)
		}
		os.Exit(0)
	}

	if os.Getenv("TUNNELIX_ASKPASS") == "1" {
		os.Exit(askpass.RunHelper())
	}
	os.Exit(m.Run())
}

type sshServer struct {
	listener net.Listener
	mu       sync.Mutex
	conns    map[*ssh.ServerConn]bool
	sign     ssh.Signer
	public   ssh.PublicKey
}

func startServer(t *testing.T, public ssh.PublicKey) *sshServer {
	t.Helper()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	sign, _ := ssh.NewSignerFromKey(private)
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	s := &sshServer{listener: l, conns: map[*ssh.ServerConn]bool{}, sign: sign, public: public}
	go s.serve()
	t.Cleanup(func() { l.Close(); s.drop() })
	return s
}
func (s *sshServer) drop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		c.Close()
	}
}
func (s *sshServer) serve() {
	for {
		c, e := s.listener.Accept()
		if e != nil {
			return
		}
		go s.handle(c)
	}
}
func (s *sshServer) handle(raw net.Conn) {
	defer raw.Close()
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if c.User() == "tester" && string(p) == "correct-password" {
			return nil, nil
		}
		return nil, fmt.Errorf("denied")
	}, PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if s.public != nil && bytes.Equal(s.public.Marshal(), k.Marshal()) {
			return nil, nil
		}
		return nil, fmt.Errorf("denied")
	}}
	cfg.AddHostKey(s.sign)
	conn, channels, requests, e := ssh.NewServerConn(raw, cfg)
	if e != nil {
		return
	}
	s.mu.Lock()
	s.conns[conn] = true
	s.mu.Unlock()
	defer func() { conn.Close(); s.mu.Lock(); delete(s.conns, conn); s.mu.Unlock() }()
	var listeners []net.Listener
	var lm sync.Mutex
	listenersClosed := false
	defer func() {
		lm.Lock()
		defer lm.Unlock()
		listenersClosed = true
		for _, l := range listeners {
			l.Close()
		}
	}()
	go func() {
		for r := range requests {
			switch r.Type {
			case "tcpip-forward":
				var p struct {
					Address string
					Port    uint32
				}
				if ssh.Unmarshal(r.Payload, &p) != nil {
					r.Reply(false, nil)
					continue
				}
				l, e := net.Listen("tcp", net.JoinHostPort(p.Address, strconv.Itoa(int(p.Port))))
				if e != nil {
					r.Reply(false, nil)
					continue
				}
				lm.Lock()
				if listenersClosed {
					lm.Unlock()
					l.Close()
					r.Reply(false, nil)
					continue
				}
				listeners = append(listeners, l)
				lm.Unlock()
				r.Reply(true, nil)
				go func() {
					for {
						c, e := l.Accept()
						if e != nil {
							return
						}
						go func() {
							payload := struct {
								Address    string
								Port       uint32
								Origin     string
								OriginPort uint32
							}{p.Address, p.Port, "127.0.0.1", 12345}
							ch, req, e := conn.OpenChannel("forwarded-tcpip", ssh.Marshal(payload))
							if e != nil {
								c.Close()
								return
							}
							go ssh.DiscardRequests(req)
							bridge(ch, c)
						}()
					}
				}()
			default:
				r.Reply(false, nil)
			}
		}
	}()
	for ch := range channels {
		if ch.ChannelType() != "direct-tcpip" {
			ch.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		var p struct {
			Host       string
			Port       uint32
			Origin     string
			OriginPort uint32
		}
		if ssh.Unmarshal(ch.ExtraData(), &p) != nil {
			ch.Reject(ssh.ConnectionFailed, "bad request")
			continue
		}
		dst, e := net.Dial("tcp", net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port))))
		if e != nil {
			ch.Reject(ssh.ConnectionFailed, "unreachable")
			continue
		}
		c, req, e := ch.Accept()
		if e != nil {
			dst.Close()
			continue
		}
		go ssh.DiscardRequests(req)
		go bridge(c, dst)
	}
}
func bridge(ch ssh.Channel, c net.Conn) {
	defer ch.Close()
	defer c.Close()
	done := make(chan struct{})
	go func() { io.Copy(ch, c); ch.CloseWrite(); close(done) }()
	io.Copy(c, ch)
	c.Close()
	<-done
}
func freePort(t *testing.T) int {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func echoServer(t *testing.T) int {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}
func harness(t *testing.T, s *sshServer) (*Manager, config.Tunnel, string) {
	t.Helper()
	_, e := exec.LookPath("ssh")
	if e != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("OpenSSH required in CI")
		}
		t.Skip("OpenSSH required")
	}
	dir := t.TempDir()
	known := filepath.Join(dir, "known_hosts")
	cfg := filepath.Join(dir, "ssh_config")
	null := os.DevNull
	content := fmt.Sprintf("Host *\n UserKnownHostsFile \"%s\"\n GlobalKnownHostsFile \"%s\"\n IdentitiesOnly yes\n IdentityAgent none\n", filepath.ToSlash(known), filepath.ToSlash(null))
	if e = os.WriteFile(cfg, []byte(content), 0600); e != nil {
		t.Fatal(e)
	}
	helper, _ := os.Executable()
	wrapper := filepath.Join(dir, "test-ssh")
	if runtime.GOOS == "windows" {
		wrapper += ".exe"
	}
	if e = os.Link(helper, wrapper); e != nil {
		data, err := os.ReadFile(helper)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(wrapper, data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if actual := os.Getenv("TUNNELIX_TEST_HELPER"); actual != "" {
		helper = actual
	}
	m := New(helper)
	t.Cleanup(m.Close)
	tunnel := config.Tunnel{ID: "test", Name: "Integration", Host: "127.0.0.1", Port: s.listener.Addr().(*net.TCPAddr).Port, User: "tester", Auth: "password", Reconnect: true, Mappings: []config.Mapping{{Type: "local", Bind: "127.0.0.1", Port: freePort(t), Target: "127.0.0.1", TargetPort: echoServer(t)}}}
	return m, tunnel, wrapper
}
func await(t *testing.T, m *Manager, id, want, secret string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	seen := map[string]bool{}
	for time.Now().Before(deadline) {
		statuses, challenges := m.Snapshot()
		for _, c := range challenges {
			if !seen[c.ID] {
				seen[c.ID] = true
				if e := m.Respond(c.ID, secret, true); e != nil {
					t.Fatal(e)
				}
			}
		}
		for _, s := range statuses {
			if s.ID == id && s.State == want {
				return
			}
			if s.State == "error" && want != "error" {
				t.Fatalf("unexpected error: %+v", s)
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	statuses, challenges := m.Snapshot()
	t.Fatalf("timeout waiting %s: %+v challenges %+v", want, statuses, challenges)
}
func echo(t *testing.T, c net.Conn) {
	t.Helper()
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	payload := []byte("tunnelix-forwarding-test")
	if _, e := c.Write(payload); e != nil {
		t.Fatal(e)
	}
	got := make([]byte, len(payload))
	if _, e := io.ReadFull(c, got); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("forwarding data mismatch")
	}
}
func TestPasswordLocalSOCKSRemoteAndReconnect(t *testing.T) {
	s := startServer(t, nil)
	m, connection, path := harness(t, s)
	connection.Network = &config.NetworkOptions{ConnectTimeout: 30, ServerAliveInterval: 10, ServerAliveCountMax: 4}
	dynamic := config.Mapping{Type: "dynamic", Bind: "127.0.0.1", Port: freePort(t)}
	remote := connection.Mappings[0]
	remote.Type = "remote"
	remote.Port = freePort(t)
	connection.Mappings = append(connection.Mappings, dynamic, remote)
	if e := m.Connect(connection, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, connection.ID, "connected", "correct-password")
	for _, port := range []int{connection.Mappings[0].Port, remote.Port} {
		c, e := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if e != nil {
			t.Fatal(e)
		}
		echo(t, c)
	}
	d, e := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(dynamic.Port)), nil, proxy.Direct)
	if e != nil {
		t.Fatal(e)
	}
	c, e := d.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(remote.TargetPort)))
	if e != nil {
		t.Fatal(e)
	}
	echo(t, c)
	s.drop()
	await(t, m, connection.ID, "reconnecting", "correct-password")
	await(t, m, connection.ID, "connected", "correct-password")
	m.Disconnect(connection.ID)
	await(t, m, connection.ID, "disconnected", "")
	if m.Active(connection.ID) {
		t.Fatal("session remains active")
	}
	for _, f := range connection.Mappings {
		if f.Type == "remote" {
			continue
		}
		l, e := net.Listen("tcp", net.JoinHostPort(f.Bind, strconv.Itoa(f.Port)))
		if e != nil {
			t.Fatal("port not released", e)
		}
		l.Close()
	}
	statuses, _ := m.Snapshot()
	for _, s := range statuses {
		for _, l := range s.Logs {
			if strings.Contains(l.Code, "correct-password") {
				t.Fatal("secret in logs")
			}
		}
	}
}
func TestWrongPasswordStopsRetry(t *testing.T) {
	s := startServer(t, nil)
	m, c, path := harness(t, s)
	if e := m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "error", "wrong-password")
	statuses, _ := m.Snapshot()
	if statuses[0].Code != "AUTH_FAILED" {
		t.Fatalf("wrong failure: %+v", statuses)
	}
	m.Disconnect(c.ID)
	if m.Active(c.ID) {
		t.Fatal("authentication failure retried")
	}
}
func TestEncryptedPrivateKey(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	public, _ := ssh.NewPublicKey(pub)
	s := startServer(t, public)
	m, c, path := harness(t, s)
	block, e := ssh.MarshalPrivateKeyWithPassphrase(key, "integration", []byte("key-passphrase"))
	if e != nil {
		t.Fatal(e)
	}
	c.Identity = filepath.Join(t.TempDir(), "identity")
	os.WriteFile(c.Identity, pem.EncodeToMemory(block), 0600)
	c.Auth = "key"
	if e = m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "connected", "key-passphrase")
	m.Disconnect(c.ID)
}
func TestDisconnectDuringAuthentication(t *testing.T) {
	s := startServer(t, nil)
	m, c, path := harness(t, s)
	if e := m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, p := m.Snapshot()
		if len(p) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() { m.Disconnect(c.ID); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect hung during authentication")
	}
	_, p := m.Snapshot()
	if len(p) != 0 {
		t.Fatal("authentication prompts leaked")
	}
}
func TestPortConflictIsNotConnected(t *testing.T) {
	s := startServer(t, nil)
	m, c, path := harness(t, s)
	occupied, e := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Mappings[0].Port)))
	if e != nil {
		t.Fatal(e)
	}
	defer occupied.Close()
	c.Reconnect = false
	if e = m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "error", "correct-password")
	statuses, _ := m.Snapshot()
	if statuses[0].Code != "PORT_IN_USE" {
		t.Fatal(statuses)
	}
}

func TestChangedHostKeyRequiresIntervention(t *testing.T) {
	s := startServer(t, nil)
	m, c, path := harness(t, s)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	sign, _ := ssh.NewSignerFromKey(other)
	known := filepath.Join(filepath.Dir(path), "known_hosts")
	os.WriteFile(known, []byte(fmt.Sprintf("[127.0.0.1]:%d %s", c.Port, ssh.MarshalAuthorizedKey(sign.PublicKey()))), 0600)
	if e := m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "error", "correct-password")
	statuses, prompts := m.Snapshot()
	if statuses[0].Code != "HOST_KEY_FAILED" || len(prompts) != 0 {
		t.Fatalf("unexpected host handling: %+v %+v", statuses, prompts)
	}
}
func TestDuplicateConnectAndRejectedTrust(t *testing.T) {
	s := startServer(t, nil)
	m, c, path := harness(t, s)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := m.Connect(c, path); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	deadline := time.Now().Add(5 * time.Second)
	rejected := false
	for time.Now().Before(deadline) {
		_, prompts := m.Snapshot()
		if len(prompts) > 0 {
			if len(prompts) != 1 || prompts[0].Kind != "host" {
				t.Fatal(prompts)
			}
			m.Respond(prompts[0].ID, "", false)
			rejected = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !rejected {
		t.Fatal("no host trust request")
	}
	await(t, m, c.ID, "error", "")
	statuses, _ := m.Snapshot()
	if statuses[0].Code != "AUTH_CANCELLED" {
		t.Fatal(statuses)
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(path), "known_hosts")); !os.IsNotExist(e) {
		t.Fatal("rejected host was persisted")
	}
}

func waitTestFinished(t *testing.T, m *Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for m.Active(id) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if m.Active(id) {
		t.Fatal("test session still active")
	}
}
func TestDraftConnectionCleansPortsAndCredentials(t *testing.T) {
	server := startServer(t, nil)
	m, draft, path := harness(t, server)
	draft.Network = &config.NetworkOptions{ConnectTimeout: 20, ServerAliveInterval: 30, ServerAliveCountMax: 2}
	draft.Name = "" // A connection test does not require a display name.
	originalID := draft.ID
	remote := draft.Mappings[0]
	remote.Type = "remote"
	remote.Port = freePort(t)
	draft.Mappings = append(draft.Mappings, remote, config.Mapping{Type: "dynamic", Bind: "127.0.0.1", Port: freePort(t)})
	id, e := m.StartTest(draft, path)
	if e != nil {
		t.Fatal(e)
	}
	if id == originalID {
		t.Fatal("test reused saved tunnel ID")
	}
	await(t, m, id, "testPassed", "correct-password")
	waitTestFinished(t, m, id)
	statuses, prompts := m.Snapshot()
	if len(statuses) != 1 || !statuses[0].IsTest || len(prompts) != 0 {
		t.Fatal(statuses, prompts)
	}
	for _, mapping := range draft.Mappings {
		if mapping.Type == "remote" {
			continue
		}
		listener, e := net.Listen("tcp", net.JoinHostPort(mapping.Bind, strconv.Itoa(mapping.Port)))
		if e != nil {
			t.Fatal("test did not release port", e)
		}
		listener.Close()
	}
	m.mu.Lock()
	if len(m.sessions) != 0 {
		t.Error("test retained credential session")
	}
	m.mu.Unlock()
	m.CancelTest(id, true)
	statuses, _ = m.Snapshot()
	if len(statuses) != 0 {
		t.Fatal("test result not removed")
	}
	// A fresh test must request a password again, even after a successful test.
	id, e = m.StartTest(draft, path)
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(5 * time.Second)
	asked := false
	for time.Now().Before(deadline) {
		_, prompts = m.Snapshot()
		if len(prompts) > 0 {
			if prompts[0].Kind != "password" {
				t.Fatal(prompts)
			}
			asked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !asked {
		t.Fatal("fresh test did not request credentials")
	}
	m.CancelTest(id, false)
	waitTestFinished(t, m, id)
	statuses, prompts = m.Snapshot()
	if statuses[0].State != "testCancelled" || len(prompts) != 0 {
		t.Fatal(statuses, prompts)
	}
}
func TestDraftFailureDoesNotRetry(t *testing.T) {
	server := startServer(t, nil)
	m, draft, path := harness(t, server)
	id, e := m.StartTest(draft, path)
	if e != nil {
		t.Fatal(e)
	}
	await(t, m, id, "error", "wrong-password")
	waitTestFinished(t, m, id)
	statuses, _ := m.Snapshot()
	if statuses[0].Code != "AUTH_FAILED" || statuses[0].Attempt != 1 {
		t.Fatal(statuses)
	}
}
func TestDraftTimeoutAndSingleFlight(t *testing.T) {
	server := startServer(t, nil)
	m, draft, path := harness(t, server)
	id, e := m.startTest(draft, path, 100*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.StartTest(draft, path); e == nil || e.Error() != "TEST_IN_PROGRESS" {
		t.Fatal("concurrent test accepted", e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		statuses, _ := m.Snapshot()
		if len(statuses) > 0 && statuses[0].State == "error" {
			if statuses[0].Code != "TEST_TIMEOUT" {
				t.Fatal(statuses)
			}
			waitTestFinished(t, m, id)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("test timeout did not stop SSH")
}
func TestCancelTestCannotStopRegularConnection(t *testing.T) {
	server := startServer(t, nil)
	m, connection, path := harness(t, server)
	if e := m.Connect(connection, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, connection.ID, "connected", "correct-password")
	m.CancelTest(connection.ID, true)
	if !m.Active(connection.ID) {
		t.Fatal("test cancellation stopped normal connection")
	}
	m.Disconnect(connection.ID)
}

func waitConnectedAfter(t *testing.T, m *Manager, id, previousSince string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		statuses, _ := m.Snapshot()
		for _, s := range statuses {
			if s.ID == id && s.State == "connected" && s.Since != previousSince {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("connection did not recover")
}
func TestResumeRecoversDesiredSessionsOnly(t *testing.T) {
	server := startServer(t, nil)
	m, c, path := harness(t, server)
	if e := m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "connected", "correct-password")
	before, _ := m.Snapshot()
	// Make the state timestamp observably distinct without relying on real sleep.
	m.mu.Lock()
	status := m.statuses[c.ID]
	status.Since = "before-resume"
	m.statuses[c.ID] = status
	m.mu.Unlock()
	m.RecoverNetwork()
	waitConnectedAfter(t, m, c.ID, "before-resume")
	_, prompts := m.Snapshot()
	if len(prompts) > 0 {
		t.Fatal("resume forgot session credentials")
	}
	m.Disconnect(c.ID)
	m.RecoverNetwork()
	if m.Active(c.ID) {
		t.Fatal("resume resurrected stopped tunnel")
	}
	if before[0].State != "connected" {
		t.Fatal(before)
	}
}
func TestResumeHonorsReconnectDisabled(t *testing.T) {
	server := startServer(t, nil)
	m, c, path := harness(t, server)
	c.Reconnect = false
	if e := m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "connected", "correct-password")
	m.mu.Lock()
	s := m.sessions[c.ID]
	m.mu.Unlock()
	m.RecoverNetwork()
	if len(s.wake) != 0 {
		t.Fatal("non-reconnecting tunnel was scheduled for restart")
	}
	m.Disconnect(c.ID)
}
func TestBatchConnectionsAndRapidStop(t *testing.T) {
	server := startServer(t, nil)
	m, base, path := harness(t, server)
	// Trust/authenticate once, then explicitly answer each independent session.
	if e := m.Connect(base, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, base.ID, "connected", "correct-password")
	m.Disconnect(base.ID)
	const count = 8
	connections := make([]config.Tunnel, count)
	for i := range connections {
		c := base
		c.ID = fmt.Sprintf("batch-%d", i)
		c.Mappings = append([]config.Mapping{}, base.Mappings...)
		c.Mappings[0].Port = freePort(t)
		connections[i] = c
		if e := m.Connect(c, path); e != nil {
			t.Fatal(e)
		}
	}
	for _, c := range connections {
		await(t, m, c.ID, "connected", "correct-password")
	}
	var stops sync.WaitGroup
	for _, c := range connections {
		stops.Add(1)
		go func() { defer stops.Done(); m.Disconnect(c.ID) }()
	}
	stops.Wait()
	for _, c := range connections {
		if m.Active(c.ID) {
			t.Fatal("batch disconnect left session")
		}
		l, e := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Mappings[0].Port)))
		if e != nil {
			t.Fatal("batch port remains bound", e)
		}
		l.Close()
	}
	for range 15 {
		if e := m.Connect(base, path); e != nil {
			t.Fatal(e)
		}
		m.Disconnect(base.ID)
	}
	m.mu.Lock()
	remaining := len(m.sessions)
	m.mu.Unlock()
	if remaining != 0 {
		t.Fatal("rapid stop leaked sessions", remaining)
	}
}
func TestLongOutageBackoffCancellation(t *testing.T) {
	server := startServer(t, nil)
	m, c, path := harness(t, server)
	server.listener.Close()
	if e := m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(20 * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		statuses, _ := m.Snapshot()
		for _, s := range statuses {
			if s.State == "reconnecting" {
				attempt = s.Attempt
			}
		}
		if attempt >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if attempt < 3 {
		t.Fatal("outage did not back off")
	}
	start := time.Now()
	m.Disconnect(c.ID)
	if time.Since(start) > time.Second {
		t.Fatal("backoff did not cancel promptly")
	}
	m.RecoverNetwork()
	if m.Active(c.ID) {
		t.Fatal("stopped outage retried")
	}
}

func TestSilentNetworkBlackholeRecovers(t *testing.T) {
	server := startServer(t, nil)
	m, c, path := harness(t, server)
	gate, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	var blocked atomic.Bool
	var sockets sync.Map
	acceptDone := make(chan struct{})
	defer func() {
		gate.Close()
		<-acceptDone
		sockets.Range(func(key, value any) bool { key.(net.Conn).Close(); return true })
	}()
	go func() {
		defer close(acceptDone)
		for {
			client, e := gate.Accept()
			if e != nil {
				return
			}
			upstream, e := net.Dial("tcp", server.listener.Addr().String())
			if e != nil {
				client.Close()
				continue
			}
			sockets.Store(client, true)
			sockets.Store(upstream, true)
			relay := func(dst, src net.Conn) {
				defer src.Close()
				defer dst.Close()
				defer sockets.Delete(src)
				buffer := make([]byte, 32768)
				for {
					n, e := src.Read(buffer)
					if n > 0 && !blocked.Load() {
						if _, e = dst.Write(buffer[:n]); e != nil {
							return
						}
					}
					if e != nil {
						return
					}
				}
			}
			go relay(client, upstream)
			go relay(upstream, client)
		}
	}()
	c.Port = gate.Addr().(*net.TCPAddr).Port
	if e = m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "connected", "correct-password")
	blocked.Store(true)
	// Exercise the production 15-second keepalive policy, not a shortened test policy.
	deadline := time.Now().Add(80 * time.Second)
	detected := false
	for time.Now().Before(deadline) {
		statuses, _ := m.Snapshot()
		for _, s := range statuses {
			if s.ID == c.ID && s.State == "reconnecting" {
				detected = true
			}
		}
		if detected {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !detected {
		t.Fatal("silent network loss was never detected")
	}
	blocked.Store(false)
	m.RecoverNetwork()
	await(t, m, c.ID, "connected", "correct-password")
	connection, e := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Mappings[0].Port)))
	if e != nil {
		t.Fatal(e)
	}
	echo(t, connection)
	m.Disconnect(c.ID)
}
func TestConnectionSoak(t *testing.T) {
	durationText := os.Getenv("TUNNELIX_SOAK_DURATION")
	if durationText == "" {
		t.Skip("set TUNNELIX_SOAK_DURATION=2m (or longer) for traffic soak")
	}
	duration, e := time.ParseDuration(durationText)
	if e != nil || duration < time.Second {
		t.Fatal("invalid soak duration")
	}
	server := startServer(t, nil)
	m, c, path := harness(t, server)
	if e = m.Connect(c, path); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "connected", "correct-password")
	deadline := time.Now().Add(duration)
	requests := 0
	for time.Now().Before(deadline) {
		conn, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Mappings[0].Port)), time.Second)
		if e != nil {
			t.Fatal(e)
		}
		echo(t, conn)
		requests++
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("transferred %d verified payloads over %s", requests, duration)
	m.Close()
	if m.Active(c.ID) {
		t.Fatal("soak shutdown leaked session")
	}
}

func TestSuppliedPasswordConnectsWithoutPasswordPrompt(t *testing.T) {
	server := startServer(t, nil)
	m, c, path := harness(t, server)
	if e := m.ConnectWithSecret(c, path, "correct-password"); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		statuses, prompts := m.Snapshot()
		for _, p := range prompts {
			if p.Kind != "host" {
				t.Fatal("supplied password was not used")
			}
			if e := m.Respond(p.ID, "", true); e != nil {
				t.Fatal(e)
			}
		}
		for _, status := range statuses {
			if status.State == "error" {
				t.Fatal(status.Code)
			}
			if status.State == "connected" {
				conn, e := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Mappings[0].Port)))
				if e != nil {
					t.Fatal(e)
				}
				echo(t, conn)
				m.Disconnect(c.ID)
				return
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("saved password connection timed out")
}
func TestSuppliedWrongPasswordStops(t *testing.T) {
	server := startServer(t, nil)
	m, c, path := harness(t, server)
	if e := m.ConnectWithSecret(c, path, "wrong-password"); e != nil {
		t.Fatal(e)
	}
	await(t, m, c.ID, "error", "")
	statuses, _ := m.Snapshot()
	if statuses[0].Code != "SAVED_AUTH_FAILED" || statuses[0].Attempt != 1 {
		t.Fatal(statuses)
	}
}
