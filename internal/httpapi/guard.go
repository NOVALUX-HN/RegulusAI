package httpapi

// NOV-82 Paso 62: DNS-rebinding + network-exposure guard for the dashboard.
//
// Handler() stays a bare mux (existing tests pin it); main wraps it with
// Guard. The guard is deny-by-default on identity, open on loopback
// transport: Host is always validated (rebinding defense), Origin and
// Sec-Fetch-Site are validated when present (browser-only signals),
// and a Bearer token is required only for non-loopback binds.
import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// GuardConfig bounds one Guard. Bind/Port are the configured --bind/--port;
// Token is the required Bearer token ("" with RequireAuth denies all,
// fail-closed); RequireAuth is true exactly for non-loopback binds.
type GuardConfig struct {
	Bind        string
	Port        int
	Token       string
	RequireAuth bool
}

func guardLoopbackName(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

func guardSplitHostPort(h string) (host, port string) {
	if hh, pp, err := net.SplitHostPort(h); err == nil {
		return hh, pp
	}
	return h, ""
}

// Guard wraps next with the host/origin/token policy. Denials are 403,
// except a missing or wrong Bearer token on a guarded bind, which is 401
// with a WWW-Authenticate challenge.
func Guard(next http.Handler, cfg GuardConfig) http.Handler {
	allowed := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	if cfg.Bind != "" && !guardLoopbackName(cfg.Bind) {
		allowed[strings.ToLower(cfg.Bind)] = true
	}
	wantPort := ""
	if cfg.Port > 0 {
		wantPort = strconv.Itoa(cfg.Port)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Host check, always: a rebinding page makes the browser send
		// the attacker's Host to 127.0.0.1, so only loopback names (plus
		// the explicit bind) pass, with the configured port when present.
		h, p := guardSplitHostPort(r.Host)
		h = strings.ToLower(strings.Trim(h, "[]"))
		if !allowed[h] || (p != "" && wantPort != "" && p != wantPort) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		// 2. Origin / Referer when present: must match this host:port.
		// Absent (curl, non-browser clients) passes.
		for _, key := range []string{"Origin", "Referer"} {
			if v := r.Header.Get(key); v != "" {
				u, err := url.Parse(v)
				if err != nil {
					http.Error(w, "forbidden origin", http.StatusForbidden)
					return
				}
				oh, op := strings.ToLower(u.Hostname()), u.Port()
				if oh != h || op != p {
					http.Error(w, "forbidden origin", http.StatusForbidden)
					return
				}
			}
		}
		// 3. Sec-Fetch-Site when present: only same-origin/same-site/none.
		if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
			switch sfs {
			case "same-origin", "same-site", "none":
			default:
				http.Error(w, "forbidden fetch site", http.StatusForbidden)
				return
			}
		}
		// 4. Bearer on guarded (non-loopback) binds, constant-time.
		if cfg.RequireAuth {
			got := ""
			if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
				got = strings.TrimPrefix(ah, "Bearer ")
			}
			if cfg.Token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(cfg.Token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="regulus dashboard"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
