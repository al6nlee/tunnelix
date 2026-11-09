package askpass

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrokerRejectsWrongToken(t *testing.T) {
	var called atomic.Int32
	b, e := Start(context.Background(), func(context.Context, string, string) (string, error) { called.Add(1); return "secret", nil })
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	c, e := dial(b.address)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	json.NewEncoder(c).Encode(Request{Token: "invalid", Prompt: "Password:"})
	var response Response
	if json.NewDecoder(c).Decode(&response) == nil || called.Load() != 0 {
		t.Fatal("unauthenticated request reached handler")
	}
}
func TestBrokerRoundTripAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b, e := Start(ctx, func(ctx context.Context, p, h string) (string, error) {
		if p == "wait" {
			<-ctx.Done()
			return "", ctx.Err()
		}
		return "test-password", nil
	})
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	c, e := dial(b.address)
	if e != nil {
		t.Fatal(e)
	}
	c.SetDeadline(time.Now().Add(time.Second))
	json.NewEncoder(c).Encode(Request{Token: b.token, Prompt: "Password:"})
	var r Response
	if e = json.NewDecoder(c).Decode(&r); e != nil || !r.OK || r.Value != "test-password" {
		t.Fatal(r, e)
	}
	c.Close()
	cancel()
}
