//go:build ignore

// Generate an offline update signing key. Never commit the private output.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run scripts/update-keygen.go /absolute/private-key-file")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	f, err := os.OpenFile(os.Args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic(err)
	}
	_, err = f.WriteString(base64.StdEncoding.EncodeToString(priv))
	if err != nil {
		f.Close()
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
	fmt.Println("Public key:", base64.StdEncoding.EncodeToString(pub))
}
