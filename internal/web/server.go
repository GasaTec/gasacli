// Package web serves the local, embedded pre-alpha dashboard.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/sessions"
)

//go:embed static/*
var assets embed.FS

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Options configures the loopback listener. Zero Host and Port use the defaults.
type Options struct {
	Host           string
	Port           int
	Fallback       bool
	Home           string
	SessionRoots   sessions.Roots
	CurrentProject string
	WebMode        string
	Sender         sessions.Sender
}

// Server owns a loopback listener and its in-memory session secret.
type Server struct {
	url      string
	session  *launchSession
	http     *http.Server
	done     chan struct{}
	mu       sync.Mutex
	serveErr error
}

// URL returns the public dashboard URL, safe to print.
func (s *Server) URL() string { return s.url }

// LaunchURL renews the single-use browser bootstrap code for 60 seconds,
// invalidating any previous code. It returns an empty URL if randomness fails.
// Only pass it to OpenBrowser; do not print or log it.
func (s *Server) LaunchURL() string {
	var code [32]byte
	if _, err := rand.Read(code[:]); err != nil {
		return ""
	}
	// An empty URL means no fresh code was issued; the caller must not open it.
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	s.session.code = base64.RawURLEncoding.EncodeToString(code[:])
	s.session.expires = s.session.now().Add(60 * time.Second)
	s.session.used = false
	return s.url + "#code=" + s.session.code
}

// Done is closed when the listener stops serving. It is nil only for a nil Server,
// and a nil channel never fires.
func (s *Server) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.done
}

// ServeError returns the listener's unexpected Serve error, or nil while
// serving or after a normal shutdown.
func (s *Server) ServeError() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serveErr
}

func (s *Server) serve(listener net.Listener) {
	err := s.http.Serve(listener)
	s.mu.Lock()
	if err != http.ErrServerClosed {
		s.serveErr = err
	}
	s.mu.Unlock()
	close(s.done)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// Start binds before probing, avoiding a probe/bind race on the preferred port.
// The caller must call Shutdown; cancellation of ctx also closes an owned listener.
func Start(ctx context.Context, options Options) (*Server, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host := options.Host
	if host == "" {
		host = "127.0.0.1"
	}
	switch host {
	case "127.0.0.1", "::1", "localhost":
	default:
		return nil, fmt.Errorf("dashboard host %q must be loopback (127.0.0.1, ::1 or localhost)", host)
	}
	// Resolve localhost ourselves so system DNS cannot redirect the bind.
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port := options.Port
	if port == 0 {
		port = 3199
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid dashboard port %d", port)
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		endpoint := "http://" + address
		if healthy(ctx, endpoint) {
			if !options.Fallback {
				return nil, fmt.Errorf("dashboard responds on %s, but the existing instance could not be confirmed; port fallback is disabled", address)
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !options.Fallback {
			return nil, fmt.Errorf("dashboard cannot listen on %s (port may be occupied): %w", address, err)
		}
		listener, err = net.Listen("tcp", net.JoinHostPort(host, "0"))
		if err != nil {
			return nil, fmt.Errorf("dashboard loopback fallback: %w", err)
		}
	}
	endpoint := "http://" + listener.Addr().String()
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("generate dashboard session secret: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	var code [32]byte
	if _, err := rand.Read(code[:]); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("generate dashboard launch code: %w", err)
	}
	launchCode := base64.RawURLEncoding.EncodeToString(code[:])
	actualPort := listener.Addr().(*net.TCPAddr).Port
	if options.CurrentProject == "" {
		options.CurrentProject, _ = os.Getwd()
	}
	session := &launchSession{secret: token, code: launchCode, expires: time.Now().Add(60 * time.Second), now: time.Now}
	srv := &Server{url: endpoint + "/", session: session, done: make(chan struct{})}
	srv.http = &http.Server{
		Handler:           handler(endpoint, actualPort, token, session, options),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// SSE keeps one response open; per-response write deadlines would close it.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}
	go srv.serve(listener)
	go func() {
		select {
		case <-ctx.Done():
			_ = srv.http.Close()
		case <-srv.done:
		}
	}()
	return srv, nil
}

func healthy(ctx context.Context, endpoint string) bool {
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/api/health", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var health struct {
		App     string `json:"app"`
		Version string `json:"version"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	return err == nil && len(body) <= 4096 && json.Unmarshal(body, &health) == nil && health.App == "gasacli" && health.Version == "pre-alpha"
}

// authenticateAPI protects all routes by default, including future non-API routes.
func authenticateAPI(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, assetErr := assets.ReadFile(strings.TrimPrefix(r.URL.Path, "/"))
		public := r.URL.Path == "/" || (strings.HasPrefix(r.URL.Path, "/static/") && assetErr == nil) || r.URL.Path == "/api/health" || (r.URL.Path == "/api/session" && r.Method == http.MethodPost)
		if !public {
			valid := func(candidate string) bool {
				return token != "" && subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1
			}
			authorized := valid(r.Header.Get("X-Gasa-Token"))
			if cookie, err := r.Cookie("gasa_session"); err == nil {
				authorized = valid(cookie.Value) || authorized
			}
			if !authorized {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type launchSession struct {
	mu           sync.Mutex
	secret, code string
	expires      time.Time
	now          func() time.Time
	used         bool
}

func (s *launchSession) exchange(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Code string `json:"code"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	valid := subtle.ConstantTimeCompare([]byte(payload.Code), []byte(s.code)) == 1
	if !valid || s.used || !s.now().Before(s.expires) {
		s.mu.Unlock()
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.used = true
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "gasa_session", Value: s.secret, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{}`)
}

func handler(origin string, port int, token string, session *launchSession, options ...Options) http.Handler {
	home := ""
	var roots sessions.Roots
	currentProject, webMode := "", "auto"
	var sender sessions.Sender
	if len(options) > 0 {
		home, roots = options[0].Home, options[0].SessionRoots
		currentProject, webMode = options[0].CurrentProject, options[0].WebMode
		sender = options[0].Sender
	}
	if webMode == "" {
		if saved, err := config.Load(); err == nil {
			webMode = saved.Web.Mode
		} else {
			webMode = "auto"
		}
	}
	initialMode := webMode
	loadMode := func() string {
		if saved, err := config.Load(); err == nil {
			return saved.Web.Mode
		}
		return initialMode
	}
	cache := &snapshotCache{home: home, roots: roots, currentProject: currentProject}
	getSnapshot := cache.get
	allowed := map[string]bool{}
	for _, host := range []string{"127.0.0.1", "localhost", "::1"} {
		allowed[net.JoinHostPort(host, strconv.Itoa(port))] = true
	}
	routes := authenticateAPI(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/session" && r.Method == http.MethodPost {
			session.exchange(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.URL.Path == "/api/settings" && r.Method == http.MethodPost {
				var payload struct {
					WebMode string `json:"webMode"`
				}
				decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
				if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF {
					http.Error(w, "invalid settings", http.StatusBadRequest)
					return
				}
				if err := config.SaveWebMode(home, payload.WebMode); err != nil {
					http.Error(w, "could not save dashboard mode", http.StatusBadRequest)
					return
				}
				jsonResponse(w, map[string]string{"webMode": payload.WebMode})
				return
			}
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/health":
			w.Header().Set("Content-Type", "application/json")
			if r.Method != http.MethodHead {
				_, _ = io.WriteString(w, `{"app":"gasacli","version":"pre-alpha"}`)
			}
		case "/api/settings":
			jsonResponse(w, map[string]string{"webMode": loadMode()})
		case "/api/capabilities":
			capabilities := sessions.Capabilities{}
			if sender != nil {
				capabilities = sender.Capabilities()
			}
			jsonResponse(w, capabilities)
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method != http.MethodHead {
				content, _ := assets.ReadFile("static/index.html")
				_, _ = w.Write(content)
			}
		default:
			if strings.HasPrefix(r.URL.Path, "/static/") {
				name := strings.TrimPrefix(r.URL.Path, "/")
				if strings.Contains(name, "..") || strings.Contains(name, "\\") {
					http.NotFound(w, r)
					return
				}
				content, err := assets.ReadFile(name)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				switch {
				case strings.HasSuffix(name, ".css"):
					w.Header().Set("Content-Type", "text/css; charset=utf-8")
				case strings.HasSuffix(name, ".svg"):
					w.Header().Set("Content-Type", "image/svg+xml")
				default:
					w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
				}
				if r.Method != http.MethodHead {
					_, _ = w.Write(content)
				}
				return
			}
			if apiHandler(getSnapshot, w, r) {
				return
			}
			http.NotFound(w, r)
		}
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if !allowed[r.Host] || (r.Method != http.MethodGet && r.Method != http.MethodHead && (len(r.Header.Values("Origin")) != 1 || r.Header.Get("Origin") != origin)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		routes.ServeHTTP(w, r)
	})
}
