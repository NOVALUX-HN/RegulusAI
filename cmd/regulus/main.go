// Command regulus is the Regulus AI core CLI: the compiled owner of gauntlet
// evidence, spec validation, lesson library and budget state.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

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
func serve(args []string) int {
	port := httpapi.DefaultDashboardPort
	bind := "127.0.0.1"
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Println("usage: regulus serve [--port N] [--bind ADDR]")
			return 0
		}
		if v, ok := strings.CutPrefix(a, "--port="); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 1 || n > 65535 {
				fmt.Fprintln(os.Stderr, "usage: regulus serve [--port N] [--bind ADDR]")
				return 2
			}
			port = n
			continue
		}
		if v, ok := strings.CutPrefix(a, "--bind="); ok {
			bind = strings.TrimSpace(v)
			if bind == "" || net.ParseIP(bind) == nil {
				fmt.Fprintf(os.Stderr, "regulus serve: invalid bind address %q\n", bind)
				return 2
			}
			continue
		}
		fmt.Fprintln(os.Stderr, "usage: regulus serve [--port N] [--bind ADDR] (use --port=N --bind=ADDR)")
		return 2
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	srv := httpapi.New(version, func() doctor.Result {
		return doctor.Probe(context.Background(), doctor.DefaultRunner())
	}, gates.ShRunner{})
	fmt.Printf("regulus dashboard: http://localhost:%d/ (bind %s, read-only)\n", port, addr)
	if warn := nonLoopbackWarning(bind); warn != "" {
		fmt.Fprintln(os.Stderr, warn)
	}
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "regulus serve: %v\n", err)
		return 1
	}
	return 0
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
