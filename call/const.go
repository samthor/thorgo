package call

import (
	"time"
)

const (
	SocketCodeUnknownProtocol = 3000
	SocketCodeExcessTraffic   = 3001
	SocketCodeBadCallId       = 3002

	helloTimeout = time.Second * 10 // how long to allow for initial handshake
	noopTimeout  = time.Second * 6  // send a no-op roughly every ~seconds
)
