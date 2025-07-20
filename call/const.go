package call

import (
	"time"
)

const (
	SocketCodeUnknownProtocol = 3999
	SocketCodeExcessTraffic   = 3998
	SocketCodeBadCallID       = 3997

	helloTimeout = time.Second * 10 // how long to allow for initial handshake
	noopTimeout  = time.Second * 6  // send a no-op roughly every ~seconds
)
