// Package config holds gosfs server configuration and its CLI flag parsing.
package config

import (
	"flag"
	"fmt"
	"time"
)

// Defaults shared with the CLI.
const (
	DefaultBindAddr      = "0.0.0.0"
	DefaultPort          = 2690
	DefaultMaxUploadSize = 16 << 20 // 16MiB
	DefaultReadTimeout   = 10 * time.Second
	DefaultWriteTimeout  = 10 * time.Second
)

// DupMode selects how an upload collision with an existing file is handled.
type DupMode string

const (
	DupError     DupMode = "error"
	DupOverwrite DupMode = "overwrite"
	DupRename    DupMode = "rename"
)

// Config is the resolved runtime configuration for the server.
type Config struct {
	RootDir       string
	BindAddr      string
	Port          int
	MaxUploadSize int64
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	DirsFirst     bool
	DupMode       DupMode
}

// Parse builds a Config from command-line args (excluding the program name).
func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("gosfs", flag.ContinueOnError)
	var (
		rootDir       string
		bindAddr      string
		listenPort    int
		maxUploadSize int64
		dirsFirst     bool
		dupMode       string
	)
	fs.StringVar(&rootDir, "root-dir", "/tmp/gosfs", "root directory")
	fs.StringVar(&bindAddr, "bind-addr", DefaultBindAddr, "IP address to bind")
	fs.IntVar(&listenPort, "port", DefaultPort, "port number to listen on")
	fs.Int64Var(&maxUploadSize, "max-size", DefaultMaxUploadSize, "max size of uploaded file (byte)")
	fs.BoolVar(&dirsFirst, "dirs-first", false, "list directories before files")
	fs.StringVar(&dupMode, "dup", string(DupError), "duplicate upload: error|overwrite|rename")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return &Config{
		RootDir:       rootDir,
		BindAddr:      bindAddr,
		Port:          listenPort,
		MaxUploadSize: maxUploadSize,
		ReadTimeout:   DefaultReadTimeout,
		WriteTimeout:  DefaultWriteTimeout,
		DirsFirst:     dirsFirst,
		DupMode:       DupMode(dupMode),
	}, nil
}

// ListenAddr returns the host:port address the server binds.
func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.BindAddr, c.Port)
}
