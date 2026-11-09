package askpass

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type Request struct {
	Token  string `json:"token"`
	Prompt string `json:"prompt"`
	Hint   string `json:"hint"`
}
type Response struct {
	Value string `json:"value"`
	OK    bool   `json:"ok"`
}
type Handler func(context.Context, string, string) (string, error)
type Broker struct {
	address, token string
	listener       net.Listener
	cleanup        func()
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

func RandomID() string {
	var b [24]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func Start(ctx context.Context, handle Handler) (*Broker, error) {
	l, address, cleanup, e := listen()
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(ctx)
	b := &Broker{address: address, token: RandomID(), listener: l, cleanup: cleanup, cancel: cancel}
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			b.wg.Add(1)
			go func() {
				defer b.wg.Done()
				defer c.Close()
				c.SetDeadline(time.Now().Add(130 * time.Second))
				done := make(chan struct{})
				defer close(done)
				go func() {
					select {
					case <-ctx.Done():
						c.Close()
					case <-done:
					}
				}()
				var r Request
				if json.NewDecoder(io.LimitReader(c, 16384)).Decode(&r) != nil || subtle.ConstantTimeCompare([]byte(r.Token), []byte(b.token)) != 1 {
					return
				}
				if len(r.Prompt) > 8192 {
					return
				}
				requestCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
				defer cancel()
				value, err := handle(requestCtx, r.Prompt, r.Hint)
				json.NewEncoder(c).Encode(Response{Value: value, OK: err == nil})
			}()
		}
	}()
	return b, nil
}
func (b *Broker) Env(helper string) []string {
	return []string{"TUNNELIX_ASKPASS=1", "TUNNELIX_ASKPASS_ADDRESS=" + b.address, "TUNNELIX_ASKPASS_TOKEN=" + b.token, "SSH_ASKPASS=" + helper, "SSH_ASKPASS_REQUIRE=force", "DISPLAY=tunnelix:0"}
}
func (b *Broker) Close() { b.cancel(); b.listener.Close(); b.wg.Wait(); b.cleanup() }

// RunHelper must execute before creating the desktop application. Only stdout carries the response.
func RunHelper() int {
	if len(os.Args) < 2 {
		return 1
	}
	c, e := dial(os.Getenv("TUNNELIX_ASKPASS_ADDRESS"))
	if e != nil {
		return 1
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(125 * time.Second))
	e = json.NewEncoder(c).Encode(Request{Token: os.Getenv("TUNNELIX_ASKPASS_TOKEN"), Prompt: os.Args[1], Hint: os.Getenv("SSH_ASKPASS_PROMPT")})
	if e != nil {
		return 1
	}
	var r Response
	if json.NewDecoder(io.LimitReader(c, 32768)).Decode(&r) != nil || !r.OK || strings.ContainsAny(r.Value, "\r\n\x00") {
		return 1
	}
	fmt.Fprintln(os.Stdout, r.Value)
	return 0
}
func Kind(prompt, hint string) (string, error) {
	p := strings.ToLower(prompt)
	if hint == "confirm" || strings.Contains(p, "are you sure you want to continue connecting") {
		return "host", nil
	}
	if strings.Contains(p, "enter passphrase for key") {
		return "passphrase", nil
	}
	if strings.Contains(p, "password:") || strings.Contains(p, "password: ") {
		return "password", nil
	}
	return "", errors.New("UNSUPPORTED_AUTH_PROMPT")
}
