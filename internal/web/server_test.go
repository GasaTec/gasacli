package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/sessions"
	"github.com/GasaTec/gasacli/internal/trust"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func start(t *testing.T, options Options) *Server {
	t.Helper()
	s, err := Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Drop idle keep-alive client connections so Shutdown does not wait on them.
		http.DefaultTransport.(*http.Transport).CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return s
}

func get(t *testing.T, endpoint string) string {
	t.Helper()
	u, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal("invalid test endpoint")
	}
	u.Fragment = ""
	endpoint = u.String()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s: status %d", endpoint, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func serverEndpoint(t *testing.T, s *Server) (string, string) {
	t.Helper()
	u, err := url.Parse(s.LaunchURL())
	if err != nil {
		t.Fatal("invalid dashboard URL")
	}
	if !strings.HasPrefix(u.Fragment, "code=") || u.RawQuery != "" || u.Path != "/" || strings.Contains(s.URL(), "#") {
		t.Fatal("missing fragment credential or unexpected query/path")
	}
	token := s.session.secret
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		t.Fatal("session secret must contain 32 random bytes")
	}
	decoded, err = base64.RawURLEncoding.DecodeString(strings.TrimPrefix(u.Fragment, "code="))
	if err != nil || len(decoded) != 32 || strings.TrimPrefix(u.Fragment, "code=") == token {
		t.Fatal("launch code must contain 32 independent random bytes")
	}
	return u.Scheme + "://" + u.Host, token
}

func TestPreferredPortAndEmbeddedDashboard(t *testing.T) {
	port := freePort(t)
	s := start(t, Options{Port: port})
	endpoint, _ := serverEndpoint(t, s)
	if want := "http://127.0.0.1:" + strconv.Itoa(port); endpoint != want {
		t.Fatalf("endpoint = %s, want %s", endpoint, want)
	}
	body := get(t, s.URL())
	for _, text := range []string{"GasaCLI", "Seu workspace", "Mudanças", "Envio para um agente ativo está planejado"} {
		if !strings.Contains(body, text) {
			t.Errorf("dashboard missing %q", text)
		}
	}
	if static := get(t, s.URL()+"static/bootstrap.js"); !strings.Contains(static, "/api/session") {
		t.Fatal("public embedded bootstrap asset unavailable before cookie exchange")
	}
	logoResp, err := http.Get(s.URL() + "static/mark.svg")
	if err != nil {
		t.Fatal(err)
	}
	logoResp.Body.Close()
	if logoResp.Header.Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("logo content type = %q", logoResp.Header.Get("Content-Type"))
	}
	var health map[string]string
	if err := json.Unmarshal([]byte(get(t, endpoint+"/api/health")), &health); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(health, map[string]string{"app": "gasacli", "version": "pre-alpha"}) {
		t.Fatalf("health = %#v", health)
	}
	if s.http.ReadHeaderTimeout != 5*time.Second || s.http.ReadTimeout == 0 || s.http.WriteTimeout != 0 {
		t.Fatal("missing HTTP timeouts")
	}
}

func TestOccupiedNonHTTPPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	if _, err := Start(context.Background(), Options{Port: port}); err == nil || !strings.Contains(err.Error(), "cannot listen") {
		t.Fatalf("want clear occupied-port error, got %v", err)
	}
	s := start(t, Options{Port: port, Fallback: true})
	endpoint, _ := serverEndpoint(t, s)
	if strings.HasSuffix(endpoint, ":"+strconv.Itoa(port)) {
		t.Fatal("reused occupied port")
	}
	get(t, s.URL())
}

func TestOtherHTTPAppNotReused(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"app":"other","version":"pre-alpha"}`)
	}))
	defer other.Close()
	u, _ := url.Parse(other.URL)
	port, _ := strconv.Atoi(u.Port())
	if _, err := Start(context.Background(), Options{Port: port}); err == nil {
		t.Fatal("reused other app")
	}
	s := start(t, Options{Port: port, Fallback: true})
	endpoint, _ := serverEndpoint(t, s)
	if endpoint == other.URL {
		t.Fatal("reused other app")
	}
	get(t, s.URL())
	if body := get(t, other.URL); !strings.Contains(body, "other") {
		t.Fatal("external app was changed")
	}
}

func TestExistingInstanceRequiresAuthenticationOrFallback(t *testing.T) {
	port := freePort(t)
	owner := start(t, Options{Port: port})
	endpoint, ownerToken := serverEndpoint(t, owner)
	if _, err := Start(context.Background(), Options{Port: port}); err == nil || !strings.Contains(err.Error(), "existing instance could not be confirmed; port fallback is disabled") || strings.Contains(err.Error(), ownerToken) {
		t.Fatal("expected clear, secret-free authentication error")
	}
	fallback := start(t, Options{Host: "localhost", Port: port, Fallback: true})
	fallbackEndpoint, fallbackToken := serverEndpoint(t, fallback)
	if fallbackEndpoint == endpoint || fallbackToken == ownerToken || fallback.http == nil {
		t.Fatal("fallback must own a different listener and secret")
	}
	if err := fallback.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	get(t, endpoint+"/api/health")
}

func TestHTTPGuards(t *testing.T) {
	const origin = "http://127.0.0.1:3199"
	h := handler(origin, 3199, "test-session", nil)
	for _, tc := range []struct {
		name, method, host, origin, path string
		status                           int
	}{
		{"IPv4", "GET", "127.0.0.1:3199", "", "/", 200},
		{"localhost", "GET", "localhost:3199", "", "/", 200},
		{"IPv6", "GET", "[::1]:3199", "", "/", 200},
		{"rebind", "GET", "attacker.example:3199", "", "/", 403},
		{"wrong port", "GET", "127.0.0.1:3200", "", "/api/health", 403},
		{"missing port", "GET", "localhost", "", "/", 403},
		{"bad origin", "POST", "127.0.0.1:3199", "https://evil.example", "/", 403},
		{"missing origin", "POST", "127.0.0.1:3199", "", "/", 403},
		{"null origin", "POST", "127.0.0.1:3199", "null", "/", 403},
		{"matching origin", "POST", "127.0.0.1:3199", origin, "/", 405},
		{"preflight", "OPTIONS", "127.0.0.1:3199", "https://evil.example", "/", 403},
		{"head", "HEAD", "127.0.0.1:3199", "", "/", 200},
		{"health head", "HEAD", "127.0.0.1:3199", "", "/api/health", 200},
		{"unknown", "GET", "127.0.0.1:3199", "", "/static/../server.go", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, origin+tc.path, nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
			for key, want := range map[string]string{"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "Content-Security-Policy": contentSecurityPolicy, "Referrer-Policy": "no-referrer"} {
				if got := w.Header().Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("CORS allowed")
			}
			if tc.method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD response has body")
			}
		})
	}
	r := httptest.NewRequest("POST", origin, nil)
	r.Header["Origin"] = []string{origin, "https://evil.example"}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("accepted duplicate Origin headers")
	}
}

func TestRejectNonLoopbackAndInvalidPorts(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", "192.168.0.1", "example.com", "127.0.0.2", "localhost.evil"} {
		if _, err := Start(context.Background(), Options{Host: host}); err == nil {
			t.Errorf("accepted host %q", host)
		}
	}
	for _, port := range []int{-1, 65536} {
		if _, err := Start(context.Background(), Options{Port: port}); err == nil {
			t.Errorf("accepted port %d", port)
		}
	}
}

func TestContextCancellationClosesOwnedListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := Start(ctx, Options{Port: freePort(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())
	cancel()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("listener survived cancellation")
	}
	if _, err := Start(ctx, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Start: %v", err)
	}
}

func TestHealthHandshakeRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{`not json`, `{"app":"gasacli","version":"other"}`, `{"app":"gasacli","version":"pre-alpha"} {}`, strings.Repeat(" ", 4096) + `{"app":"gasacli","version":"pre-alpha"}`, `{"app":"gasacli","version":"pre-alpha"}` + strings.Repeat(" ", 4096)} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
			defer s.Close()
			if healthy(context.Background(), s.URL) {
				t.Fatal("accepted invalid handshake")
			}
		})
	}
	target := httptest.NewServer(handler("http://127.0.0.1:3199", 3199, "test-session", nil))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	if healthy(context.Background(), redirect.URL) {
		t.Fatal("accepted redirect")
	}
}

func TestOpenBrowser(t *testing.T) {
	endpoint := "http://127.0.0.1:3199"
	called := false
	err := OpenBrowser(endpoint, func(name string, args ...string) error {
		called = true
		wantName, wantArgs := browserCommand(runtime.GOOS, endpoint)
		if name != wantName || !reflect.DeepEqual(args, wantArgs) {
			t.Fatalf("launcher = %s %v", name, args)
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("OpenBrowser: called=%v err=%v", called, err)
	}
	sentinel := errors.New("launcher failed")
	if err := OpenBrowser(endpoint, func(string, ...string) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("lost launcher error: %v", err)
	}
	for _, tc := range []struct {
		platform, name string
		args           []string
	}{
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", endpoint}},
		{"darwin", "open", []string{endpoint}},
		{"linux", "xdg-open", []string{endpoint}},
	} {
		name, args := browserCommand(tc.platform, endpoint)
		if name != tc.name || !reflect.DeepEqual(args, tc.args) {
			t.Errorf("%s: %s %v", tc.platform, name, args)
		}
	}
}

func TestAPIAuthentication(t *testing.T) {
	const token = "test-session-secret"
	h := authenticateAPI(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, tc := range []struct {
		name, path, header, cookie string
		status                     int
	}{
		{"missing", "/api/data", "", "", 401},
		{"wrong header", "/api/data", "wrong", "", 401},
		{"wrong cookie", "/api/data", "", "wrong", 401},
		{"header", "/api/data", token, "", 200},
		{"cookie", "/api/data", "", token, 200},
		{"header or cookie", "/api/data", "wrong", token, 200},
		{"cookie or header", "/api/data", token, "wrong", 200},
		{"query ignored", "/api/data?token=" + token + "&X-Gasa-Token=" + token + "&gasa_session=" + token, "", "", 401},
		{"nested API", "/api/jobs/1", "", "", 401},
		{"outside API", "/new-route", "", "", 401},
		{"outside API cookie", "/new-route", "", token, 200},
		{"root public", "/", "", "", 200},
		{"session GET protected", "/api/session", "", "", 401},
		{"health public", "/api/health", "", "", 200},
		{"health child protected", "/api/health/details", "", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3199"+tc.path, nil)
			r.Header.Set("X-Gasa-Token", tc.header)
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "gasa_session", Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
		})
	}
}

func TestSessionCookieAndSecretNotDisclosed(t *testing.T) {
	var logs bytes.Buffer
	previousLog := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousLog)
	s := start(t, Options{Port: freePort(t)})
	endpoint, token := serverEndpoint(t, s)
	client := &http.Client{Timeout: 2 * time.Second}
	root, err := client.Get(s.URL())
	if err != nil {
		t.Fatal("root request failed")
	}
	root.Body.Close()
	if root.StatusCode != 200 || len(root.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("public root must not issue a cookie")
	}
	request, _ := http.NewRequest(http.MethodPost, endpoint+"/api/session", strings.NewReader(`{"code":"`+s.session.code+`"}`))
	request.Header.Set("Origin", endpoint)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("session request failed")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(response.Cookies()) != 1 {
		t.Fatal("session exchange failed")
	}
	session := response.Cookies()[0]
	for _, path := range []string{"/", "/api/health", "/api/missing", "/api/missing?token=" + token, "/missing"} {
		resp, err := client.Get(endpoint + path)
		if err != nil {
			t.Fatal("dashboard request failed")
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal("reading dashboard response failed")
		}
		if bytes.Contains(body, []byte(token)) || bytes.Contains(body, []byte(s.session.code)) {
			t.Fatal("secret or code disclosed in response body")
		}
		if len(resp.Header.Values("Set-Cookie")) != 0 {
			t.Fatal("session must only be issued by authenticated exchange")
		}
		if path == "/" && resp.StatusCode != http.StatusOK {
			t.Fatal("root must remain public")
		}
		if strings.HasPrefix(path, "/api/missing") && resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated API: status %d", resp.StatusCode)
		}
		if path == "/api/health" && string(body) != `{"app":"gasacli","version":"pre-alpha"}` {
			t.Fatal("health must contain only public compatibility data")
		}
	}
	if session == nil || session.Value != token || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || session.Path != "/" || session.Domain != "" || session.MaxAge != 0 || !session.Expires.IsZero() {
		t.Fatal("missing or invalid host-only session cookie")
	}
	for _, useHeader := range []bool{false, true} {
		r, _ := http.NewRequest(http.MethodGet, endpoint+"/api/missing", nil)
		if useHeader {
			r.Header.Set("X-Gasa-Token", token)
		} else {
			r.AddCookie(session)
		}
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal("authenticated request failed")
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("authenticated request must reach router: %d", resp.StatusCode)
		}
	}
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), s.session.code) {
		t.Fatal("secret or code disclosed in captured server logs")
	}
}

func TestConcurrentStartsHaveIndependentSecrets(t *testing.T) {
	port := freePort(t)
	type result struct {
		server *Server
		err    error
	}
	results := make(chan result, 2)
	ready := make(chan struct{})
	for range 2 {
		go func() {
			<-ready
			s, err := Start(context.Background(), Options{Port: port, Fallback: true})
			results <- result{s, err}
		}()
	}
	close(ready)
	var endpoints, tokens []string
	for range 2 {
		r := <-results
		if r.err != nil {
			t.Error(r.err)
			continue
		}
		t.Cleanup(func() { _ = r.server.Shutdown(context.Background()) })
		endpoint, token := serverEndpoint(t, r.server)
		endpoints = append(endpoints, endpoint)
		tokens = append(tokens, token)
	}
	if t.Failed() {
		t.FailNow()
	}
	if endpoints[0] == endpoints[1] || tokens[0] == tokens[1] {
		t.Fatal("concurrent instances shared endpoint or secret")
	}
	for i, endpoint := range endpoints {
		r, _ := http.NewRequest(http.MethodGet, endpoint+"/api/missing", nil)
		r.Header.Set("X-Gasa-Token", tokens[1-i])
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal("cross-instance request failed")
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatal("accepted another instance's secret")
		}
	}
}

func TestSessionExchange(t *testing.T) {
	const origin = "http://127.0.0.1:3199"
	now := time.Now()
	for _, tc := range []struct {
		name, body, origin, host string
		expired, used            bool
		status                   int
	}{
		{"valid", `{"code":"launch-code"}`, origin, "127.0.0.1:3199", false, false, 200},
		{"wrong", `{"code":"wrong"}`, origin, "127.0.0.1:3199", false, false, 401},
		{"empty", `{}`, origin, "127.0.0.1:3199", false, false, 401},
		{"malformed", `{`, origin, "127.0.0.1:3199", false, false, 401},
		{"trailing JSON", `{"code":"launch-code"} {}`, origin, "127.0.0.1:3199", false, false, 401},
		{"oversized", `{"code":"` + strings.Repeat("x", 1024) + `"}`, origin, "127.0.0.1:3199", false, false, 401},
		{"expired", `{"code":"launch-code"}`, origin, "127.0.0.1:3199", true, false, 401},
		{"used", `{"code":"launch-code"}`, origin, "127.0.0.1:3199", false, true, 401},
		{"bad origin", `{"code":"launch-code"}`, "https://evil.example", "127.0.0.1:3199", false, false, 403},
		{"missing origin", `{"code":"launch-code"}`, "", "127.0.0.1:3199", false, false, 403},
		{"bad host", `{"code":"launch-code"}`, origin, "evil.example:3199", false, false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &launchSession{secret: "session-secret", code: "launch-code", expires: now.Add(time.Minute), now: func() time.Time { return now }, used: tc.used}
			if tc.expired {
				state.expires = now
			}
			h := handler(origin, 3199, state.secret, state)
			r := httptest.NewRequest(http.MethodPost, origin+"/api/session", strings.NewReader(tc.body))
			r.Host = tc.host
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d", w.Code, tc.status)
			}
			if strings.Contains(w.Body.String(), state.secret) || strings.Contains(w.Body.String(), state.code) {
				t.Fatal("credential in response body")
			}
			if tc.status != 200 && len(w.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("failed exchange issued cookie")
			}
			if tc.status == 200 {
				cookies := w.Result().Cookies()
				if len(cookies) != 1 || cookies[0].Value != state.secret || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
					t.Fatal("invalid session cookie")
				}
				r = httptest.NewRequest(http.MethodPost, origin+"/api/session", strings.NewReader(tc.body))
				r.Header.Set("Origin", origin)
				w = httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 401 {
					t.Fatal("launch code reused")
				}
			}
		})
	}
}

func TestConcurrentSessionExchangeIsSingleUse(t *testing.T) {
	state := &launchSession{secret: "secret", code: "code", expires: time.Now().Add(time.Minute), now: time.Now}
	results := make(chan int, 2)
	for range 2 {
		go func() {
			w := httptest.NewRecorder()
			state.exchange(w, httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(`{"code":"code"}`)))
			results <- w.Code
		}()
	}
	first, second := <-results, <-results
	if !((first == 200 && second == 401) || (first == 401 && second == 200)) {
		t.Fatal("code must be consumed exactly once")
	}
}

func TestServeErrorObservable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	s := &Server{http: &http.Server{}, done: make(chan struct{})}
	go s.serve(listener)
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not exit")
	}
	if s.ServeError() == nil {
		t.Fatal("Serve error discarded")
	}
	if err := (*Server)(nil).Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := (&Server{}).Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	owned := start(t, Options{Port: freePort(t)})
	if err := owned.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-owned.done
	if owned.ServeError() != nil {
		t.Fatal("normal shutdown recorded as failure")
	}
}

func TestDashboardUsesExternalScriptsAndStrictCSP(t *testing.T) {
	content, err := assets.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "<script>") || !strings.Contains(string(content), `src="/static/bootstrap.js"`) {
		t.Fatal("dashboard scripts must be external")
	}
	if !strings.Contains(contentSecurityPolicy, "script-src 'self'") || strings.Contains(contentSecurityPolicy, "unsafe-inline") || strings.Contains(contentSecurityPolicy, "https:") {
		t.Fatal("CSP must allow same-origin resources only")
	}
	bootstrap, err := assets.ReadFile("static/bootstrap.js")
	if err != nil || !bytes.Contains(bootstrap, []byte("history.replaceState")) || !bytes.Contains(bootstrap, []byte("fetch('/api/session'")) {
		t.Fatal("bootstrap must remove fragment before exchanging the single-use code")
	}
	app, err := assets.ReadFile("static/app.js")
	if err != nil || bytes.Contains(app, []byte("innerHTML")) {
		t.Fatal("frontend must build DOM without assigning untrusted HTML")
	}
}

func TestSessionAPITrustRedactionAndTraversal(t *testing.T) {
	project := filepath.Join(t.TempDir(), "trusted")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	home, claude := t.TempDir(), t.TempDir()
	if err := trust.NewStore(home).Grant(project); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(claude, "project", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	fixture := `{"type":"user","sessionId":"private-id","cwd":"` + filepath.ToSlash(project) + `","message":{"role":"user","content":"<script>alert(1)</script> sk-secret-value"}}`
	if err := os.WriteFile(file, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Start(context.Background(), Options{Port: freePort(t), Home: home, SessionRoots: sessions.Roots{Claude: claude}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	})
	call := func(path string) (int, []byte) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, s.URL()+strings.TrimPrefix(path, "/"), nil)
		req.Header.Set("X-Gasa-Token", s.session.secret)
		resp, e := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
	code, body := call("/api/projects")
	if code != 200 || !bytes.Contains(body, []byte(`"name":"trusted"`)) || bytes.Contains(body, []byte("sk-secret-value")) {
		t.Fatalf("trusted project response invalid: status=%d body=%s", code, body)
	}
	projects := buildSnapshot(home, sessions.Roots{Claude: claude}).projects
	if len(projects) != 1 || len(projects[0].Sessions) != 1 {
		t.Fatalf("unexpected projects: %#v", projects)
	}
	code, body = call("/api/sessions/" + projects[0].Sessions[0].ID)
	if code != 200 || bytes.Contains(body, []byte("sk-secret-value")) || bytes.Contains(body, []byte("<script>")) {
		t.Fatalf("session detail leaked unsafe input: status=%d body=%s", code, body)
	}
	code, _ = call("/api/git/status?worktree=../../etc")
	if code != 404 {
		t.Fatalf("traversal worktree status = %d, want 404", code)
	}
	code, _ = call("/api/sessions/../../etc/passwd")
	if code == 200 {
		t.Fatal("path traversal session id was accepted")
	}
}

func TestLaunchCodeAuthenticatesProjectsAndSSE(t *testing.T) {
	home, project := t.TempDir(), filepath.Join(t.TempDir(), "trusted-project")
	claude, codex := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := trust.NewStore(home).Grant(project); err != nil {
		t.Fatal(err)
	}
	state := &launchSession{secret: "session-secret", code: "single-use-code", expires: time.Now().Add(time.Minute), now: time.Now}
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	address := server.Listener.Addr().(*net.TCPAddr)
	port := address.Port
	origin := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	server.Config.Handler = handler(origin, port, "session-secret", state, Options{Home: home, SessionRoots: sessions.Roots{Claude: claude, Codex: codex}, CurrentProject: project})
	server.Start()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	login, err := http.NewRequest(http.MethodPost, server.URL+"/api/session", strings.NewReader(`{"code":"single-use-code"}`))
	if err != nil {
		t.Fatal(err)
	}
	login.Header.Set("Origin", server.URL)
	login.Header.Set("Content-Type", "application/json")
	response, err := client.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d", response.StatusCode)
	}
	response, err = client.Get(server.URL + "/api/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	var capabilities sessions.Capabilities
	if err := json.NewDecoder(response.Body).Decode(&capabilities); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || capabilities.Send {
		t.Fatalf("default sender capabilities = %#v, status=%d", capabilities, response.StatusCode)
	}
	configText := "web:\n  mode: auto\n  host: 127.0.0.1\n  port: 3199\n  fallback_port: true\n"
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(configText), 0600); err != nil {
		t.Fatal(err)
	}
	settings, _ := http.NewRequest(http.MethodPost, server.URL+"/api/settings", strings.NewReader(`{"webMode":"manual"}`))
	settings.Header.Set("Origin", server.URL)
	settings.Header.Set("Content-Type", "application/json")
	response, err = client.Do(settings)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("save dashboard mode status=%d", response.StatusCode)
	}
	configData, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil || !bytes.Contains(configData, []byte("mode: manual")) {
		t.Fatalf("dashboard mode was not persisted: %v", err)
	}
	response, err = client.Get(server.URL + "/api/projects")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"current":true`)) || !bytes.Contains(body, []byte(`"sessionCount":0`)) {
		t.Fatalf("trusted empty current project response status=%d body=%s", response.StatusCode, body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("events response status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || line != "event: projects\n" {
		t.Fatalf("first SSE event line = %q, %v", line, err)
	}
	data, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(data, "trusted-project") {
		t.Fatalf("SSE project data = %q, %v", data, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "\n" {
		t.Fatalf("SSE event terminator = %q, %v", line, err)
	}
	done := make(chan error, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err == nil && line == "event: projects\n" {
			return
		}
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("SSE stream closed after its first event: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSessionDetailPaginatesOlderMessages(t *testing.T) {
	messages := make([]sessions.Message, 120)
	for i := range messages {
		messages[i] = sessions.Message{Role: "user", Text: fmt.Sprintf("Message %03d", i)}
	}
	current := snapshot{sessions: map[string]sessions.Session{"safe-id": {ID: "safe-id", Messages: messages, MessageCount: len(messages)}}}
	get := func() snapshot { return current }
	request := func(path string) sessions.Session {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/sessions/safe-id"+path, nil)
		if !apiHandler(get, w, r) {
			t.Fatal("session route not handled")
		}
		if w.Code != 200 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var result sessions.Session
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	latest := request("")
	if len(latest.Messages) != 100 || latest.MessageOffset != 20 || latest.MessageCount != 120 || latest.Messages[0].Text != "Message 020" {
		t.Fatalf("unexpected latest page: %#v", latest)
	}
	older := request("?before=20")
	if len(older.Messages) != 20 || older.MessageOffset != 0 || older.Messages[0].Text != "Message 000" || older.Messages[19].Text != "Message 019" {
		t.Fatalf("unexpected older page: %#v", older)
	}
}

func TestLaunchURLRenewsAndExpiresAfter60Seconds(t *testing.T) {
	s := start(t, Options{Port: freePort(t)})
	now := time.Now()
	s.session.now = func() time.Time { return now }
	exchange := func(launch string) int {
		t.Helper()
		u, err := url.Parse(launch)
		if err != nil || !strings.HasPrefix(u.Fragment, "code=") {
			t.Fatal("invalid launch URL")
		}
		code := strings.TrimPrefix(u.Fragment, "code=")
		r := httptest.NewRequest(http.MethodPost, s.URL()+"api/session", strings.NewReader(`{"code":"`+code+`"}`))
		r.Header.Set("Origin", strings.TrimSuffix(s.URL(), "/"))
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, r)
		return w.Code
	}
	first, second := s.LaunchURL(), s.LaunchURL()
	if first == second {
		t.Fatal("LaunchURL must issue a fresh random code")
	}
	if exchange(first) != 401 {
		t.Fatal("previous launch code remains valid")
	}
	now = now.Add(60*time.Second - time.Nanosecond)
	if exchange(second) != 200 {
		t.Fatal("code expired before 60 seconds")
	}
	if exchange(second) != 401 {
		t.Fatal("used code accepted")
	}
	third := s.LaunchURL()
	now = now.Add(60 * time.Second)
	if exchange(third) != 401 {
		t.Fatal("code accepted at 60-second expiry")
	}
	fourth := s.LaunchURL()
	if exchange(fourth) != 200 {
		t.Fatal("renewal after expiry failed")
	}
}
