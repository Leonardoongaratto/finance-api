// Command finance-api runs the personal finance JSON API as a single local binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Leonardoongaratto/finance-api/internal/api"
	"github.com/Leonardoongaratto/finance-api/internal/store"
)

const (
	defaultAddr     = "127.0.0.1:8080"
	defaultDBPath   = "finance.db"
	shutdownTimeout = 5 * time.Second
)

type config struct {
	addr        string
	dbPath      string
	corsOrigins []string
}

var defaultCORSOrigins = []string{
	"http://127.0.0.1:5173",
	"http://localhost:5173",
	"http://127.0.0.1:4173",
	"http://localhost:4173",
}

func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{addr: defaultAddr, dbPath: defaultDBPath}
	if v := getenv("FINANCE_ADDR"); v != "" {
		cfg.addr = v
	}
	if v := getenv("FINANCE_DB"); v != "" {
		cfg.dbPath = v
	}
	if err := checkAddr(cfg.addr, getenv("FINANCE_ALLOW_ANY_ADDR") == "true"); err != nil {
		return config{}, err
	}
	origins, err := parseCORSOrigins(getenv("FINANCE_CORS_ORIGINS"))
	if err != nil {
		return config{}, err
	}
	cfg.corsOrigins = origins
	return cfg, nil
}

func checkAddr(addr string, allowAny bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("FINANCE_ADDR %q: %w", addr, err)
	}
	if allowAny || host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("FINANCE_ADDR %q must be a loopback address (127.0.0.1, ::1 or localhost): the app has no login; set FINANCE_ALLOW_ANY_ADDR=true only inside a container whose published ports are bound to the host loopback", addr)
}

func parseCORSOrigins(raw string) ([]string, error) {
	var origins []string
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		origin, err := normalizeOrigin(item)
		if err != nil {
			return nil, err
		}
		if !seen[origin] {
			seen[origin] = true
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		return defaultCORSOrigins, nil
	}
	return origins, nil
}

func normalizeOrigin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("FINANCE_CORS_ORIGINS: %q is not a valid origin (want scheme://host[:port], e.g. http://127.0.0.1:5173; \"*\" is never allowed)", value)
	}
	return u.Scheme + "://" + u.Host, nil
}

func run(ctx context.Context, cfg config, logger *slog.Logger, ready func(net.Addr)) error {
	db, err := store.Open(cfg.dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	handler := api.New(api.Config{Store: db, Now: time.Now, Logger: logger, CORSOrigins: cfg.corsOrigins})

	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	logger.Info("listening", "addr", listener.Addr().String())
	if ready != nil {
		ready(listener.Addr())
	}

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func realMain(ctx context.Context, getenv func(string) string, stderr io.Writer) int {
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	cfg, err := loadConfig(getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if err := run(ctx, cfg, logger, nil); err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := realMain(ctx, os.Getenv, os.Stderr)
	stop()
	os.Exit(code)
}
