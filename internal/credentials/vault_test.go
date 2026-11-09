package credentials

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/zalando/go-keyring"
	"os"
	"strings"
	"testing"
	"time"
)

type memoryKeys struct {
	values      map[string]string
	unavailable bool
}

func (k *memoryKeys) Get(s, u string) (string, error) {
	if k.unavailable {
		return "", errors.New("locked")
	}
	v, ok := k.values[s+u]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}
func (k *memoryKeys) Set(s, u, v string) error {
	if k.unavailable {
		return errors.New("locked")
	}
	k.values[s+u] = v
	return nil
}
func TestEncryptedPersistenceBindingAndTampering(t *testing.T) {
	keys := &memoryKeys{values: map[string]string{}}
	v := NewWithKeyring("test", keys)
	a, e := v.Seal("encrypted", "sensitive-password", "target-one")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(a.Value, "sensitive-password") {
		t.Fatal("plaintext stored")
	}
	b, e := v.Seal("encrypted", "sensitive-password", "target-one")
	if e != nil || a.Value == b.Value {
		t.Fatal("nonce reused", e)
	}
	reopened := NewWithKeyring("other-config-location", keys)
	if value, e := reopened.Open(a, "target-one"); e != nil || value != "sensitive-password" {
		t.Fatal("round trip failed", e)
	}
	if _, e := v.Open(a, "target-two"); e == nil {
		t.Fatal("cross-target ciphertext accepted")
	}
	raw, _ := base64.StdEncoding.DecodeString(a.Value)
	raw[len(raw)-1] ^= 1
	a.Value = base64.StdEncoding.EncodeToString(raw)
	if _, e := v.Open(a, "target-one"); e == nil {
		t.Fatal("modified ciphertext accepted")
	}
	delete(keys.values, keyService+b.KeyID)
	if _, e := v.Open(b, "target-one"); e == nil || e.Error() != "CREDENTIAL_KEY_MISSING" {
		t.Fatal("missing key silently recreated", e)
	}
	if len(keys.values) != 0 {
		t.Fatal("read created a new key")
	}
}
func TestNoPlaintextFallback(t *testing.T) {
	v := NewWithKeyring("test", &memoryKeys{unavailable: true})
	if r, e := v.Seal("encrypted", "secret", "scope"); r != nil || e == nil {
		t.Fatal("silently downgraded")
	}
	r, e := v.Seal("plain", "secret", "scope")
	if e != nil {
		t.Fatal(e)
	}
	if value, e := v.Open(r, "scope"); e != nil || value != "secret" {
		t.Fatal(e)
	}
}
func TestSystemKeyringRoundTrip(t *testing.T) {
	if os.Getenv("TUNNELIX_KEYRING_TEST") != "1" {
		t.Skip("opt-in OS keyring integration")
	}
	v := New(fmt.Sprintf("tunnelix-test-%d", time.Now().UnixNano()))
	defer func() {
		if e := keyring.Delete(keyService, v.keyID); e != nil && !errors.Is(e, keyring.ErrNotFound) {
			t.Error("temporary system key cleanup failed")
		}
	}()
	r, e := v.Seal("encrypted", "temporary-test-password", "test-target")
	if e != nil {
		t.Fatal(e)
	}
	if value, e := v.Open(r, "test-target"); e != nil || value != "temporary-test-password" {
		t.Fatal("system keyring round trip failed", e)
	}
}
