package main

import (
	"net"
	"net/http"
	"sync"
	"time"

	log "github.com/funkygao/log4go"
)

type subServer struct {
	*webServer

	idleConnsWg   sync.WaitGroup      // wait for all inflight http connections done
	idleConns     map[string]net.Conn // in keep-alive state http connections
	closedConnCh  chan string         // channel of remote addr
	idleConnsLock sync.Mutex
}

func newSubServer(httpAddr, httpsAddr string, maxClients int, gw *Gateway) *subServer {
	this := &subServer{
		webServer:    newWebServer("sub", httpAddr, httpsAddr, maxClients, gw),
		closedConnCh: make(chan string, 1<<10),
		idleConns:    make(map[string]net.Conn, 10000),
	}
	this.waitExitFunc = this.waitExit

	if this.httpServer != nil {
		// TODO https
		// register the http conn state machine hook
		// FIXME should distinguish pub from sub client
		this.httpServer.ConnState = func(c net.Conn, cs http.ConnState) {
			switch cs {
			case http.StateNew:
				this.idleConnsWg.Add(1)

			case http.StateActive:
				this.idleConnsLock.Lock()
				delete(this.idleConns, c.RemoteAddr().String())
				this.idleConnsLock.Unlock()

			case http.StateIdle:
				select {
				case <-this.gw.shutdownCh:
					// actively close the client safely because IO is all done
					c.Close()

				default:
					this.idleConnsLock.Lock()
					this.idleConns[c.RemoteAddr().String()] = c
					this.idleConnsLock.Unlock()
				}

			case http.StateClosed:
				log.Debug("http client[%s] closed", c.RemoteAddr())
				this.closedConnCh <- c.RemoteAddr().String()
				this.idleConnsWg.Done()
			}
		}
	}

	return this
}

func (this *subServer) waitExit(exit <-chan struct{}) {
	select {
	case <-exit:
		if this.httpServer != nil {
			// HTTP response will have "Connection: close"
			this.httpServer.SetKeepAlivesEnabled(false)

			// avoid new connections
			if err := this.httpListener.Close(); err != nil {
				log.Error("sub listener close: %v", err)
			}

			this.idleConnsLock.Lock()
			t := time.Now().Add(time.Millisecond * 100)
			for _, c := range this.idleConns {
				c.SetReadDeadline(t)
			}
			this.idleConnsLock.Unlock()

			log.Trace("sub waiting for all connected http client close")
			this.idleConnsWg.Wait()

			this.gw.wg.Done()
			log.Trace("sub http server stopped")
		}

		if this.httpsServer != nil {
			// TODO
			this.gw.wg.Done()
			log.Trace("sub https server stopped")
		}

		this.httpListener = nil
		this.tlsListener = nil
		this.httpServer = nil
		this.httpsServer = nil
		this.router = nil
	}

}
