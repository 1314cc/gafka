package gateway

import (
	"time"

	"github.com/funkygao/golib/ratelimiter"
)

// management server
type manServer struct {
	*webServer

	throttleAddTopic *ratelimiter.LeakyBuckets
}

func newManServer(httpAddr, httpsAddr string, maxClients int, gw *Gateway) *manServer {
	this := &manServer{
		webServer:        newWebServer("man", httpAddr, httpsAddr, maxClients, gw),
		throttleAddTopic: ratelimiter.NewLeakyBuckets(60, time.Minute),
	}

	return this
}
