package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.addr != "127.0.0.1:8080" || cfg.dbPath != "finance.db" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadConfigFromEnvironment(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{"FINANCE_ADDR": "localhost:9090", "FINANCE_DB": "x.db"}))
	if err != nil || cfg.addr != "localhost:9090" || cfg.dbPath != "x.db" {
		t.Fatalf("cfg = %+v, err = %v", cfg, err)
	}
}

func TestLoadConfigRejectsNonLoopbackAddresses(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:8080", ":8080", "192.168.0.10:8080", "example.com:80", "nonsense"} {
		if _, err := loadConfig(env(map[string]string{"FINANCE_ADDR": addr})); err == nil {
			t.Errorf("FINANCE_ADDR=%q must be rejected", addr)
		}
	}
}

func TestLoadConfigAnyAddressRequiresExplicitOptIn(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:8080", ":8080", "192.168.0.10:8080"} {
		cfg, err := loadConfig(env(map[string]string{"FINANCE_ADDR": addr, "FINANCE_ALLOW_ANY_ADDR": "true"}))
		if err != nil || cfg.addr != addr {
			t.Errorf("addr %q with opt-in: cfg = %+v, err = %v", addr, cfg, err)
		}
		for _, value := range []string{"", "false", "1", "TRUE", "yes"} {
			if _, err := loadConfig(env(map[string]string{"FINANCE_ADDR": addr, "FINANCE_ALLOW_ANY_ADDR": value})); err == nil {
				t.Errorf("addr %q with FINANCE_ALLOW_ANY_ADDR=%q must be rejected", addr, value)
			}
		}
	}
}

func TestLoadConfigOptInStillRejectsMalformedAddress(t *testing.T) {
	_, err := loadConfig(env(map[string]string{"FINANCE_ADDR": "nonsense", "FINANCE_ALLOW_ANY_ADDR": "true"}))
	if err == nil {
		t.Fatal("malformed address must be rejected even with the opt-in")
	}
}

func TestLoadConfigCORSOriginsDefault(t *testing.T) {
	want := []string{"http://127.0.0.1:5173", "http://localhost:5173", "http://127.0.0.1:4173", "http://localhost:4173"}
	for _, raw := range []string{"", "   ", " , "} {
		cfg, err := loadConfig(env(map[string]string{"FINANCE_CORS_ORIGINS": raw}))
		if err != nil || !reflect.DeepEqual(cfg.corsOrigins, want) {
			t.Errorf("FINANCE_CORS_ORIGINS=%q: origins = %v, err = %v", raw, cfg.corsOrigins, err)
		}
	}
}

func TestLoadConfigCORSOriginsFromEnvironment(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{"FINANCE_CORS_ORIGINS": " https://app.example.com , http://localhost:3000/ ,https://app.example.com"}))
	want := []string{"https://app.example.com", "http://localhost:3000"}
	if err != nil || !reflect.DeepEqual(cfg.corsOrigins, want) {
		t.Fatalf("origins = %v, err = %v", cfg.corsOrigins, err)
	}
}

func TestLoadConfigRejectsInvalidCORSOrigins(t *testing.T) {
	for _, raw := range []string{"*", "http://127.0.0.1:5173,*", "app.example.com", "ftp://app.example.com", "http://", "http://app.example.com/path", "http://app.example.com?x=1", "http://user@app.example.com"} {
		_, err := loadConfig(env(map[string]string{"FINANCE_CORS_ORIGINS": raw}))
		if err == nil || !strings.Contains(err.Error(), "FINANCE_CORS_ORIGINS") {
			t.Errorf("FINANCE_CORS_ORIGINS=%q: err = %v, want a clear error", raw, err)
		}
	}
}

func TestRunAnswersCORSPreflightForConfiguredOrigin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan net.Addr, 1)
	done := make(chan error, 1)
	cfg := config{
		addr:        "127.0.0.1:0",
		dbPath:      filepath.Join(t.TempDir(), "finance.db"),
		corsOrigins: []string{"http://127.0.0.1:4173"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() { done <- run(ctx, cfg, logger, func(addr net.Addr) { ready <- addr }) }()

	var addr net.Addr
	select {
	case addr = <-ready:
	case err := <-done:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not become ready")
	}

	req, err := http.NewRequest(http.MethodOptions, "http://"+addr.String()+"/api/accounts", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "http://127.0.0.1:4173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://127.0.0.1:4173" {
		t.Fatalf("preflight = %d %v", resp.StatusCode, resp.Header)
	}
	cancel()
	<-done
}

func TestRunServesHealthAndShutsDownGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan net.Addr, 1)
	done := make(chan error, 1)
	cfg := config{addr: "127.0.0.1:0", dbPath: filepath.Join(t.TempDir(), "finance.db")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() {
		done <- run(ctx, cfg, logger, func(addr net.Addr) { ready <- addr })
	}()

	var addr net.Addr
	select {
	case addr = <-ready:
	case err := <-done:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not become ready")
	}

	resp, err := http.Get("http://" + addr.String() + "/health")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"ok"`) {
		t.Fatalf("health = %d %s", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func TestRunFailsWhenDatabaseCannotBeOpened(t *testing.T) {
	cfg := config{addr: "127.0.0.1:0", dbPath: filepath.Join(t.TempDir(), "no-such-dir", "f.db")}
	err := run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunFailsWhenAddressIsTaken(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	cfg := config{addr: listener.Addr().String(), dbPath: filepath.Join(t.TempDir(), "f.db")}
	if err := run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil); err == nil {
		t.Fatal("expected listen error")
	}
}

func TestRealMainReportsConfigErrors(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := realMain(context.Background(), env(map[string]string{"FINANCE_ADDR": "0.0.0.0:8080"}), stderr)
	if code != 1 || !strings.Contains(stderr.String(), "loopback") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRealMainRunsUntilContextCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	stderr := &bytes.Buffer{}
	code := realMain(ctx, env(map[string]string{"FINANCE_ADDR": "127.0.0.1:0", "FINANCE_DB": filepath.Join(t.TempDir(), "f.db")}), stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
