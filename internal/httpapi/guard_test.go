package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// NOV-82: the guard denies by identity, never by route. Loopback binds
// need no token; cross-origin browser signals and foreign Hosts always
// fail; non-loopback binds need the exact Bearer token.
func TestGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	loop := GuardConfig{Bind: "127.0.0.1", Port: 17817}
	guarded := GuardConfig{Bind: "0.0.0.0", Port: 17817, Token: "s3cr3t", RequireAuth: true}

	cases := []struct {
		name    string
		cfg     GuardConfig
		host    string
		headers map[string]string
		want    int
	}{
		{name: "loopback exact passes", cfg: loop, host: "localhost:17817", want: 200},
		{name: "loopback 127 passes", cfg: loop, host: "127.0.0.1:17817", want: 200},
		{name: "loopback ipv6 passes", cfg: loop, host: "[::1]:17817", want: 200},
		{name: "loopback no port passes", cfg: loop, host: "localhost", want: 200},
		{name: "foreign host blocked", cfg: loop, host: "evil.com", want: 403},
		{name: "foreign host with port blocked", cfg: loop, host: "evil.com:17817", want: 403},
		{name: "wrong port blocked", cfg: loop, host: "localhost:9999", want: 403},
		{name: "same origin passes", cfg: loop, host: "localhost:17817", headers: map[string]string{"Origin": "http://localhost:17817"}, want: 200},
		{name: "cross origin blocked", cfg: loop, host: "localhost:17817", headers: map[string]string{"Origin": "http://evil.com"}, want: 403},
		{name: "cross referer blocked", cfg: loop, host: "localhost:17817", headers: map[string]string{"Referer": "http://evil.com/x"}, want: 403},
		{name: "fetch-site none passes", cfg: loop, host: "localhost:17817", headers: map[string]string{"Sec-Fetch-Site": "none"}, want: 200},
		{name: "fetch-site cross-site blocked", cfg: loop, host: "localhost:17817", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, want: 403},
		{name: "non-loopback host without token challenged", cfg: guarded, host: "0.0.0.0:17817", want: 401},
		{name: "lan host blocked by host check", cfg: guarded, host: "192.168.1.2:17817", want: 403},
		{name: "guarded wrong token unauthorized", cfg: guarded, host: "127.0.0.1:17817", headers: map[string]string{"Authorization": "Bearer wrong"}, want: 401},
		{name: "guarded right token passes", cfg: guarded, host: "127.0.0.1:17817", headers: map[string]string{"Authorization": "Bearer s3cr3t"}, want: 200},
		{name: "guarded empty token denies all", cfg: GuardConfig{Bind: "0.0.0.0", Port: 17817, RequireAuth: true}, host: "127.0.0.1:17817", headers: map[string]string{"Authorization": "Bearer s3cr3t"}, want: 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://x/v1/status", nil)
			req.Host = tc.host
			for k, v := range tc.headers {
				if k == "Host" {
					continue
				}
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			Guard(ok, tc.cfg).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("host=%q headers=%v: got %d, want %d", tc.host, tc.headers, rec.Code, tc.want)
			}
		})
	}
}
