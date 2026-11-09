package processguard

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case Argument:
			os.Exit(Run(os.Args[2:]))
		case "--guard-parent":
			helper, _ := os.Executable()
			p, e := New(helper, helper, []string{"--guard-worker"}, os.Environ())
			if e != nil {
				os.Exit(125)
			}
			p.Cmd.Stdout = os.Stdout
			p.Cmd.Stderr = os.Stderr
			if p.Start() != nil {
				os.Exit(125)
			}
			if p.Wait() != nil {
				os.Exit(1)
			}
			os.Exit(0)
		case "--guard-worker", "--guard-grandchild":
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				os.Exit(125)
			}
			fmt.Println(l.Addr().String())
			if os.Args[1] == "--guard-worker" {
				helper, _ := os.Executable()
				cmd := exec.Command(helper, "--guard-grandchild")
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if cmd.Start() != nil {
					os.Exit(125)
				}
				go cmd.Wait()
			}
			for {
				c, e := l.Accept()
				if e != nil {
					os.Exit(0)
				}
				c.Close()
			}
		}
	}
	os.Exit(m.Run())
}
func readAddresses(t *testing.T, r io.Reader) []string {
	t.Helper()
	out := make(chan string, 2)
	go func() {
		s := bufio.NewScanner(r)
		for s.Scan() {
			out <- s.Text()
		}
		close(out)
	}()
	result := []string{}
	for len(result) < 2 {
		select {
		case address, ok := <-out:
			if !ok {
				t.Fatal("worker exited before startup")
			}
			result = append(result, address)
		case <-time.After(10 * time.Second):
			t.Fatal("worker startup timeout")
		}
	}
	return result
}
func assertReleased(t *testing.T, addresses []string) {
	t.Helper()
	for _, address := range addresses {
		deadline := time.Now().Add(5 * time.Second)
		for {
			l, e := net.Listen("tcp", address)
			if e == nil {
				l.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("owned process still holds %s: %v", address, e)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
func TestForcedParentExitCleansOwnedProcessTree(t *testing.T) {
	helper, _ := os.Executable()
	parent := exec.Command(helper, "--guard-parent")
	stdout, e := parent.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = parent.Start(); e != nil {
		t.Fatal(e)
	}
	defer parent.Process.Kill()
	addresses := readAddresses(t, stdout)
	unrelated, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer unrelated.Close()
	if e = parent.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	_ = parent.Wait()
	assertReleased(t, addresses)
	c, e := net.DialTimeout("tcp", unrelated.Addr().String(), time.Second)
	if e != nil {
		t.Fatal("unrelated listener affected", e)
	}
	c.Close()
}
func TestStopClosesTreeWithoutKillingParent(t *testing.T) {
	helper, _ := os.Executable()
	p, e := New(helper, helper, []string{"--guard-worker"}, os.Environ())
	if e != nil {
		t.Fatal(e)
	}
	stdout, e := p.Cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Start(); e != nil {
		t.Fatal(e)
	}
	defer p.Stop()
	addresses := readAddresses(t, stdout)
	p.Stop()
	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("stop hung")
	}
	assertReleased(t, addresses)
}
func TestNoChildBeforeOwnershipHandshake(t *testing.T) {
	helper, _ := os.Executable()
	cmd := exec.Command(helper, Argument, helper, "--guard-worker")
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	cmd.Stdin = r
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	r.Close()
	w.Close()
	bytes, e := io.ReadAll(stdout)
	if e != nil {
		t.Fatal(e)
	}
	_ = cmd.Wait()
	if len(bytes) != 0 {
		t.Fatalf("child started without ownership: %s", bytes)
	}
}
