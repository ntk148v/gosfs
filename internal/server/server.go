// Package server wires together the gosfs HTTP handlers and middleware.
package server

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/ntk148v/gosfs/internal/config"
)

// Server holds the shared state for all handlers.
type Server struct {
	cfg           *config.Config
	logger        *log.Logger
	nextRequestID func() string
	healthy       int64
}

// New builds a Server from resolved configuration.
func New(cfg *config.Config, logger *log.Logger) *Server {
	return &Server{
		cfg:           cfg,
		logger:        logger,
		nextRequestID: func() string { return time.Now().Format("20060102150405.000000000") },
	}
}

// Handler wires routes and middleware into a single http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("POST /upload", s.upload)
	mux.HandleFunc("GET /healthz", s.healthz)
	return middlewares{s.tracing, s.logging}.apply(mux)
}

// ServeHTTP makes Server usable directly as an http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Handler().ServeHTTP(w, r)
}

type middleware func(http.Handler) http.Handler
type middlewares []middleware

func (mws middlewares) apply(hdlr http.Handler) http.Handler {
	if len(mws) == 0 {
		return hdlr
	}
	return mws[1:].apply(mws[0](hdlr))
}

func (s *Server) logging(hdlr http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func(start time.Time) {
			requestID := w.Header().Get("X-Request-Id")
			if requestID == "" {
				requestID = "unknown"
			}
			s.logger.Println(requestID, req.Method, req.URL.Path, req.RemoteAddr, req.UserAgent(), time.Since(start))
		}(time.Now())
		hdlr.ServeHTTP(w, req)
	})
}

func (s *Server) tracing(hdlr http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestID := req.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = s.nextRequestID()
		}
		w.Header().Set("X-Request-Id", requestID)
		hdlr.ServeHTTP(w, req)
	})
}

// Shutdown arranges graceful shutdown on SIGINT/SIGTERM and returns a context
// that is cancelled when the server has stopped.
func (s *Server) Shutdown(ctx context.Context, server *http.Server) context.Context {
	ctx, done := context.WithCancel(ctx)

	quit := make(chan struct{})
	go func() {
		defer done()
		<-quit
		atomic.StoreInt64(&s.healthy, 0)
		server.ErrorLog.Printf("Server is shutting down...\n")

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			server.ErrorLog.Printf("Could not gracefully shutdown the server: %s\n", err)
		}
	}()

	return ctx
}

// MarkHealthy records the startup time so /healthz reports uptime.
func (s *Server) MarkHealthy() {
	atomic.StoreInt64(&s.healthy, time.Now().UnixNano())
}
