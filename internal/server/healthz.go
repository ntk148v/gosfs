package server

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// healthz reports service readiness and uptime.
func (s *Server) healthz(w http.ResponseWriter, req *http.Request) {
	if h := atomic.LoadInt64(&s.healthy); h == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		fmt.Fprintf(w, "uptime: %s\n", time.Since(time.Unix(0, h)))
	}
}
