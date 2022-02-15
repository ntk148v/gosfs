package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	DefaultBindAddr      = "0.0.0.0"
	DefaultPort          = 2690
	DefaultMaxUploadSize = 5242880 // 5MiB
	DefaultReadTimeout   = 10 * time.Second
	DefaultWriteTimeout  = 10 * time.Second
)

type controller struct {
	logger        *log.Logger
	rootDir       string
	maxUploadSize int
	nextRequestID func() string
	healthy       int64
}

type File struct {
	Link    string
	Size    string
	ModTime string
	Name    string
}

type Dir struct {
	DisplayPath string
	Files       []File
}

func (c *controller) index() http.Handler {
	return http.FileServer(http.Dir(c.rootDir))
}

// func (c *controller) listDir() (Dir, error) {
// 	var dir Dir
// 	dir = Dir{
// 		DisplayPath: c.rootDir,
// 		Files:       []File{},
// 	}
// 	files, err := ioutil.ReadDir(c.rootDir)
// 	if err != nil {
// 		return dir, err
// 	}
// 	for _, file := range files {
// 		var f File
// 		if file.IsDir() {
// 			f.Name = file.Name() + "/"
// 			f.Link = file.Name() + "/"
// 			f.Size = ""
// 			f.ModTime = ""
// 		} else {
// 			f.Name = file.Name()
// 			f.Size = strconv.Itoa(int(file.Size()))
// 			f.ModTime = file.ModTime().String()
// 		}
// 	}
// }

func (c *controller) healthz(w http.ResponseWriter, req *http.Request) {
	if h := atomic.LoadInt64(&c.healthy); h == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		fmt.Fprintf(w, "uptime: %s\n", time.Since(time.Unix(0, h)))
	}
}

func (c *controller) logging(hdlr http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func(start time.Time) {
			requestID := w.Header().Get("X-Request-Id")
			if requestID == "" {
				requestID = "unknown"
			}
			c.logger.Println(requestID, req.Method, req.URL.Path, req.RemoteAddr, req.UserAgent(), time.Since(start))
		}(time.Now())
		hdlr.ServeHTTP(w, req)
	})
}

func (c *controller) tracing(hdlr http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestID := req.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = c.nextRequestID()
		}
		w.Header().Set("X-Request-Id", requestID)
		hdlr.ServeHTTP(w, req)
	})
}

func (c *controller) shutdown(ctx context.Context, server *http.Server) context.Context {
	ctx, done := context.WithCancel(ctx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer done()

		<-quit
		signal.Stop(quit)
		close(quit)

		atomic.StoreInt64(&c.healthy, 0)
		server.ErrorLog.Printf("Server is shutting down...\n")

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			server.ErrorLog.Fatalf("Could not gracefully shutdown the server: %s\n", err)
		}
	}()

	return ctx
}

type middleware func(http.Handler) http.Handler
type middlewares []middleware

func (mws middlewares) apply(hdlr http.Handler) http.Handler {
	if len(mws) == 0 {
		return hdlr
	}
	return mws[1:].apply(mws[0](hdlr))
}

func main() {
	var (
		rootDir       string
		bindAddr      string
		listenPort    int
		maxUploadSize int
	)
	flag.StringVar(&rootDir, "root-dir", "/tmp/gosfs", "root directory")
	flag.StringVar(&bindAddr, "bind-addr", DefaultBindAddr, "IP address to bind")
	flag.IntVar(&listenPort, "port", DefaultPort, "port number to listen on")
	flag.IntVar(&maxUploadSize, "max-size", DefaultMaxUploadSize, "max size of uploaded file (byte)")

	flag.Parse()

	logger := log.New(os.Stdout, "http: ", log.LstdFlags)
	logger.Printf("Server is starting...")

	if err := os.MkdirAll(rootDir, os.ModePerm); err != nil {
		log.Fatal("Unable to create root directory:", err)
	}

	c := &controller{
		logger:        logger,
		rootDir:       rootDir,
		nextRequestID: func() string { return strconv.FormatInt(time.Now().UnixNano(), 36) },
	}
	router := http.NewServeMux()
	router.Handle("/", c.index())
	router.HandleFunc("/test", c.listDirectory)
	router.HandleFunc("/healthz", c.healthz)

	listenAddr := fmt.Sprintf("%s:%d", bindAddr, listenPort)
	srv := &http.Server{
		Addr:         listenAddr,
		ErrorLog:     logger,
		Handler:      (middlewares{c.tracing, c.logging}).apply(router),
		ReadTimeout:  DefaultReadTimeout,
		WriteTimeout: DefaultWriteTimeout,
	}

	ctx := c.shutdown(context.Background(), srv)
	atomic.StoreInt64(&c.healthy, time.Now().UnixNano())

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		logger.Printf("Server is ready to handle requests at %q\n", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Listen: %s\n", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()
	logger.Println("Server exiting")
}
