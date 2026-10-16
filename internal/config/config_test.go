package config_test

import (
	"testing"
	"time"

	"github.com/ntk148v/gosfs/internal/config"
)

func TestParseDefaults(t *testing.T) {
	c, err := config.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.RootDir != "/tmp/gosfs" || c.Port != 2690 || c.MaxUploadSize != 16<<20 {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.ReadTimeout != 10*time.Second || c.WriteTimeout != 10*time.Second {
		t.Errorf("timeouts not defaulted: %+v", c)
	}
	if c.DirsFirst {
		t.Errorf("dirs-first should default false")
	}
}

func TestParseOverrides(t *testing.T) {
	c, err := config.Parse([]string{"-root-dir", "/srv", "-port", "8080", "-max-size", "1000", "-dirs-first"})
	if err != nil {
		t.Fatal(err)
	}
	if c.RootDir != "/srv" || c.Port != 8080 || c.MaxUploadSize != 1000 || !c.DirsFirst {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestListenAddr(t *testing.T) {
	c := &config.Config{BindAddr: "127.0.0.1", Port: 9000}
	if got := c.ListenAddr(); got != "127.0.0.1:9000" {
		t.Errorf("ListenAddr = %q", got)
	}
}
