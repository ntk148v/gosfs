package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var logger *log.Logger

const (
	DefaultReadTimeout  = 10 * time.Second
	DefaultWriteTimeout = 10 * time.Second
)

func main() {
	var (
		rootDir       string
		bindAddr      string
		listenPort    int
		maxUploadSize int
	)
	flag.StringVar(&rootDir, "root-dir", "/tmp/gosfs", "root directory")
	flag.StringVar(&bindAddr, "bind-addr", "0.0.0.0", "IP address to bind")
	flag.IntVar(&listenPort, "port", 2690, "port number to listen on")
	flag.IntVar(&maxUploadSize, "max-size", 5242880, "max size of uploaded file (byte)")

	flag.Parse()

	if err := os.MkdirAll(rootDir, os.ModePerm); err != nil {
		log.Fatal("Unable to create root directory:", err)
	}

	router := http.NewServeMux()
	router.Handle("/", http.FileServer(http.Dir(rootDir)))

	// Create context that listens for the interrupt signal from the OS
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", bindAddr, listenPort),
		Handler:      router,
		ReadTimeout:  DefaultReadTimeout,
		WriteTimeout: DefaultWriteTimeout,
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		log.Printf("Serving on HTTP port: %s:%d\n", bindAddr, listenPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen: %s\n", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	log.Println("Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}
