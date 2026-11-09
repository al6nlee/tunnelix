package askpass

import (
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"net"
	"time"
)

func listen() (net.Listener, string, func(), error) {
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return nil, "", nil, e
	}
	address := `\\.\pipe\tunnelix-` + RandomID()
	l, e := winio.ListenPipe(address, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + user.User.Sid.String() + ")", InputBufferSize: 16384, OutputBufferSize: 16384})
	return l, address, func() {}, e
}
func dial(address string) (net.Conn, error) {
	timeout := 3 * time.Second
	return winio.DialPipe(address, &timeout)
}
