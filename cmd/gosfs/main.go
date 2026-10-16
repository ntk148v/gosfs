// Command gosfs is a minimal HTTP file server for uploading and serving files.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/ntk148v/gosfs/internal/config"
	"github.com/ntk148v/gosfs/internal/server"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		log.Fatal("Unable to parse flags:", err)
	}

	logger := log.New(os.Stdout, "http: ", log.LstdFlags)
	logger.Printf("Server is starting...")

	if err := os.MkdirAll(cfg.RootDir, os.ModePerm); err != nil {
		log.Fatal("Unable to create root directory:", err)
	}

	s := server.New(cfg, logger)

	listenAddr := cfg.ListenAddr()
	srv := &http.Server{
		Addr:         listenAddr,
		ErrorLog:     logger,
		Handler:      s.Handler(),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	ctx := s.Shutdown(context.Background(), srv)
	s.MarkHealthy()

	go func() {
		logger.Printf("Server is ready to handle requests at %q\n", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Listen: %s\n", err)
		}
	}()

	<-ctx.Done()
	logger.Println("Server exiting")
}
