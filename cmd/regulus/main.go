// Command regulus is the Regulus AI core CLI: the compiled owner of gauntlet
// evidence, spec validation, lesson library and budget state.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/httpapi"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "serve" {
		os.Exit(serve(args[1:]))
	}
	c := cli.CLI{
		Version: version,
		Probe: func() doctor.Result {
			return doctor.Probe(context.Background(), doctor.DefaultRunner())
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	os.Exit(c.Run(args))
}

// serve runs the always-on visual (S1-S2): read-only dashboard on a fixed
// local port. It lives in main (not internal/cli) to avoid a cli<->httpapi
// import cycle: main already owns both sides.
// serveConfig holds the parsed `regulus serve` flags.
type serveConfig struct {
	port     int
	bind     string
	projects []string
}

// parseServeFlags validates the `regulus serve` flags. It prints usage (or
// the bind error) itself and reports the process exit code; ok=false means
// the caller must return code without serving.
func parseServeFlags(args []string) (serveConfig, int, bool) {
	cfg := serveConfig{port: httpapi.DefaultDashboardPort, bind: "127.0.0.1"}
	for i := 0; i < len(args); i++ {
		a := args[i]
		// F5: space form (--port N) normalizes into =-form so the single
		// validation below serves both spellings; a trailing bare flag is a usage error.
		if a == "--port" || a == "--bind" || a == "--projects" {
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "usage: regulus serve [--port N] [--bind ADDR] [--projects <dirA>,<dirB>]")
				return cfg, 2, false
			}
			i++
			a = a + "=" + args[i]
		}
		if a == "--help" || a == "-h" {
			fmt.Println("usage: regulus serve [--port N] [--bind ADDR] [--projects <dirA>,<dirB>]")
			return cfg, 0, false
		}
		if v, ok := strings.CutPrefix(a, "--port="); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 1 || n > 65535 {
				fmt.Fprintln(os.Stderr, "usage: regulus serve [--port N] [--bind ADDR] [--projects <dirA>,<dirB>]")
				return cfg, 2, false
			}
			cfg.port = n
			continue
		}
		if v, ok := strings.CutPrefix(a, "--bind="); ok {
			bind := strings.TrimSpace(v)
			if bind == "" || net.ParseIP(bind) == nil {
				fmt.Fprintf(os.Stderr, "regulus serve: invalid bind address %q\n", bind)
				return cfg, 2, false
			}
			cfg.bind = bind
			continue
		}
		if v, ok := strings.CutPrefix(a, "--projects="); ok {
			var dirs []string
			for _, d := range strings.Split(v, ",") {
				if d = strings.TrimSpace(d); d != "" {
					dirs = append(dirs, d)
				}
			}
			if len(dirs) == 0 {
				fmt.Fprintln(os.Stderr, "usage: regulus serve [--port N] [--bind ADDR] [--projects <dirA>,<dirB>]")
				return cfg, 2, false
			}
			cfg.projects = append(cfg.projects, dirs...)
			continue
		}
		fmt.Fprintln(os.Stderr, "usage: regulus serve [--port N] [--bind ADDR] [--projects <dirA>,<dirB>] (use --port=N --bind=ADDR --projects=dirA,dirB)")
		return cfg, 2, false
	}
	return cfg, 0, true
}

// listenAndServe announces the dashboard, warns on non-loopback binds and
// blocks serving. H4: the server carries timeouts (slowloris-grade exposure
// on non-loopback binds) and production shuts down gracefully on SIGINT/SIGTERM.
// serveFn receives the configured *http.Server; tests substitute a fake so
// the ready and the failure paths stay covered without binding any port.
func listenAndServe(cfg serveConfig, handler http.Handler, serveFn func(srv *http.Server) error) int {
	addr := net.JoinHostPort(cfg.bind, strconv.Itoa(cfg.port))
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second}
	fmt.Printf("regulus dashboard: http://localhost:%d/ (bind %s, read-only)\n", cfg.port, addr)
	if warn := nonLoopbackWarning(cfg.bind); warn != "" {
		fmt.Fprintln(os.Stderr, warn)
	}
	if err := serveFn(srv); err != nil {
		fmt.Fprintf(os.Stderr, "regulus serve: %v\n", err)
		return 1
	}
	return 0
}

func serve(args []string) int {
	cfg, code, ok := parseServeFlags(args)
	if !ok {
		return code
	}
	// P4: resolve every served root (the default "." and each
	// --projects entry) through the same auto-detection as the CLI, so
	// serving from a subdirectory shares that checkout's stores instead
	// of splitting .regulus/ per subdirectory. At the top level (or
	// outside git/regulus.json) ResolveRoot returns the input verbatim,
	// preserving today's allowlist exactly.
	roots := make([]string, 0, len(cfg.projects))
	for _, p := range cfg.projects {
		roots = append(roots, cli.ResolveRoot(p))
	}
	if len(roots) == 0 {
		roots = []string{cli.ResolveRoot(".")}
	}
	srv := httpapi.New(version, func() doctor.Result {
		return doctor.Probe(context.Background(), doctor.DefaultRunner())
	}, gates.ShRunner{}, roots...)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return listenAndServe(cfg, srv.Handler(), func(srv *http.Server) error {
		serveErr := make(chan error, 1)
		go func() { serveErr <- srv.ListenAndServe() }()
		select {
		case err := <-serveErr:
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		case <-ctx.Done():
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutCtx)
		}
	})
}

// nonLoopbackWarning reports a visible exposure warning when serve binds
// beyond loopback: the dashboard is read-only but still shows project
// data, so a LAN/public bind exposes it. Empty for loopback binds.
func nonLoopbackWarning(bind string) string {
	if ip := net.ParseIP(bind); ip != nil && ip.IsLoopback() {
		return ""
	}
	return "warning: regulus serve binds a non-loopback address (" + bind + "): the read-only dashboard exposes project data to that network; keep firewall rules tight"
}
