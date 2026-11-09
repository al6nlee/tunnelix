// Package credentials stores plaintext only by explicit choice. Encrypted values
// use AES-256-GCM; their random key lives exclusively in the OS credential store.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

type Record struct {
	Mode  string `json:"mode"`
	Value string `json:"value"`
	KeyID string `json:"keyId,omitempty"`
}
type Update struct {
	Mode  string `json:"mode"` // keep, none, plain, encrypted
	Value string `json:"value"`
}
type Info struct {
	Mode  string `json:"mode"`
	Saved bool   `json:"saved"`
}

func ValidSecret(value string) bool {
	return value != "" && len(value) <= 8192 && !strings.ContainsAny(value, "\x00\r\n")
}
func Validate(r *Record) error {
	if r == nil {
		return nil
	}
	if r.Mode == "plain" && ValidSecret(r.Value) && r.KeyID == "" {
		return nil
	}
	if r.Mode == "encrypted" && len(r.Value) <= 12000 && len(r.KeyID) == 64 {
		id, e := hex.DecodeString(r.KeyID)
		data, e2 := base64.StdEncoding.DecodeString(r.Value)
		if e == nil && len(id) == 32 && e2 == nil && len(data) >= 29 {
			return nil
		}
	}
	return errors.New("INVALID_CREDENTIAL")
}

type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, value string) error
}
type systemKeyring struct{}

func (systemKeyring) Get(s, u string) (string, error) { return keyring.Get(s, u) }
func (systemKeyring) Set(s, u, v string) error        { return keyring.Set(s, u, v) }

const keyService = "app.tunnelix.encryption"

type Vault struct {
	mu    sync.Mutex
	keyID string
	keys  Keyring
}

func New(namespace string) *Vault { return NewWithKeyring(namespace, systemKeyring{}) }
func NewWithKeyring(namespace string, keys Keyring) *Vault {
	sum := sha256.Sum256([]byte(namespace))
	return &Vault{keyID: hex.EncodeToString(sum[:]), keys: keys}
}
func (v *Vault) key(id string, create bool) ([]byte, error) {
	encoded, e := v.keys.Get(keyService, id)
	if errors.Is(e, keyring.ErrNotFound) && create {
		key := make([]byte, 32)
		if _, e = rand.Read(key); e != nil {
			return nil, errors.New("CREDENTIAL_STORE_UNAVAILABLE")
		}
		if e = v.keys.Set(keyService, id, base64.StdEncoding.EncodeToString(key)); e != nil {
			return nil, errors.New("CREDENTIAL_STORE_UNAVAILABLE")
		}
		return key, nil
	}
	if errors.Is(e, keyring.ErrNotFound) {
		return nil, errors.New("CREDENTIAL_KEY_MISSING")
	}
	if e != nil {
		return nil, errors.New("CREDENTIAL_STORE_UNAVAILABLE")
	}
	key, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil || len(key) != 32 {
		return nil, errors.New("CREDENTIAL_DECRYPT_FAILED")
	}
	return key, nil
}
func (v *Vault) Seal(mode, value, scope string) (*Record, error) {
	if !ValidSecret(value) {
		return nil, errors.New("INVALID_RESPONSE")
	}
	if mode == "plain" {
		return &Record{Mode: mode, Value: value}, nil
	}
	if mode != "encrypted" {
		return nil, errors.New("INVALID_CREDENTIAL")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	key, e := v.key(v.keyID, true)
	if e != nil {
		return nil, e
	}
	defer clear(key)
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, errors.New("CREDENTIAL_DECRYPT_FAILED")
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return nil, errors.New("CREDENTIAL_DECRYPT_FAILED")
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	data := gcm.Seal(nonce, nonce, []byte(value), []byte(scope))
	return &Record{Mode: mode, Value: base64.StdEncoding.EncodeToString(data), KeyID: v.keyID}, nil
}
func (v *Vault) Open(r *Record, scope string) (string, error) {
	if r == nil {
		return "", nil
	}
	if e := Validate(r); e != nil {
		return "", e
	}
	if r.Mode == "plain" {
		return r.Value, nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	key, e := v.key(r.KeyID, false)
	if e != nil {
		return "", e
	}
	defer clear(key)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	data, _ := base64.StdEncoding.DecodeString(r.Value)
	plain, e := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte(scope))
	if e != nil {
		return "", errors.New("CREDENTIAL_DECRYPT_FAILED")
	}
	defer clear(plain)
	return string(plain), nil
}
