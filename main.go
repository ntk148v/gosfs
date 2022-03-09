package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	DefaultBindAddr      = "0.0.0.0"
	DefaultPort          = 2690
	DefaultMaxUploadSize = 16 << 20 // 16MiB
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

func formatBytes(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func (c *controller) index(w http.ResponseWriter, r *http.Request) {
	// Ignore favicon
	if r.URL.Path == "/favicon.io" {
		return
	}
	// Get path to render subdirectories as well as root
	path := filepath.Join(c.rootDir, r.URL.Path)
	file, _ := os.Stat(path)

	// If there is file type, serve it directly
	if file != nil && !file.Mode().IsDir() {
		http.ServeFile(w, r, path)
	}
	// Collect data
	dir, err := c.listDir(path)
	if err != nil {
		c.logger.Println("Error listing files in directory", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t := template.Must(template.ParseFiles("index.html"))
	err = t.ExecuteTemplate(w, "index.html", dir)
	if err != nil {
		c.logger.Println("Error rendering index page:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *controller) upload(w http.ResponseWriter, r *http.Request) {
	// maximum upload of 16 MiB file
	r.ParseMultipartForm(int64(c.maxUploadSize))

	// Get handler for filename, size and headers
	fhs := r.MultipartForm.File["files"]
	for _, fh := range fhs {
		if fh.Size > int64(c.maxUploadSize) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		file, err := fh.Open()
		if err != nil {
			c.logger.Println("Error retrieveing the file:", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		c.logger.Printf("Uploaded file: %+v, file size: %+v, MIME header: %+v\n",
			fh.Filename, fh.Size, fh.Header)

		// Create file
		dst, err := os.OpenFile(filepath.Join(c.rootDir,
			strings.TrimPrefix(r.Referer(), r.Header.Get("Origin")),
			fh.Filename),
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			c.logger.Println("Error creating a new file:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		defer dst.Close()
		// Copy the uploaded file to the created file on the filesystem
		if _, err = io.Copy(dst, file); err != nil {
			c.logger.Println("Error copying new file", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, r.Referer(), http.StatusFound)
}

func (c *controller) listDir(root string) (Dir, error) {
	dir := Dir{
		DisplayPath: root,
		Files:       []File{},
	}
	files, err := ioutil.ReadDir(root)
	if err != nil {
		return dir, err
	}
	for _, file := range files {
		var f File
		f.ModTime = file.ModTime().Format("2006-01-02 15:04")
		if file.IsDir() {
			f.Name = file.Name() + "/"
			f.Size = "-"
		} else {
			f.Name = file.Name()
			f.Size = formatBytes(file.Size())
		}
		dir.Files = append(dir.Files, f)
	}
	return dir, nil
}

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
		maxUploadSize: maxUploadSize,
		nextRequestID: func() string { return strconv.FormatInt(time.Now().UnixNano(), 36) },
	}
	router := http.NewServeMux()
	router.HandleFunc("/", c.index)
	router.HandleFunc("/upload", c.upload)
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
