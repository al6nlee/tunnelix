//go:build !windows

package askpass

import (
	"net"
	"os"
	"path/filepath"
	"time"
)

func listen() (net.Listener, string, func(), error) {
	dir, e := os.MkdirTemp("", "tx-")
	if e != nil {
		return nil, "", nil, e
	}
	path := filepath.Join(dir, "ask.sock")
	l, e := net.Listen("unix", path)
	if e != nil {
		os.RemoveAll(dir)
		return nil, "", nil, e
	}
	if e = os.Chmod(path, 0600); e != nil {
		l.Close()
		os.RemoveAll(dir)
		return nil, "", nil, e
	}
	return l, path, func() { os.RemoveAll(dir) }, nil
}
func dial(address string) (net.Conn, error) { return net.DialTimeout("unix", address, 3*time.Second) }
