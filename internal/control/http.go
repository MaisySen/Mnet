package control

import (
	"net/http"
	"time"
)

// NewHTTPServer applies bounded ingress handling. WriteTimeout must exceed
// the 50-second map long poll. WebSocket upgrades hijack the connection;
// the relay owns its read/write deadlines after the upgrade.
func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      65 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
}
