package api

import (
	"bytes"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vicgupta/dap/internal/core"
	"github.com/vicgupta/dap/internal/web"
)

type sandbox struct {
	app     *core.App
	name    string
	root    string
	url     string
	apiBase string
	parent  string
	srv     *http.Server
}

// SyncSandboxes opens a loopback origin for each plugin that declares a UI
// and closes origins for plugins that no longer do.
func (s *Server) SyncSandboxes() {
	home := s.App.Home()
	want := map[string]bool{}
	for _, p := range home.Plugins {
		if p.UIRoute == "" {
			continue
		}
		root, ok := s.App.PluginUI(p.Name)
		if !ok {
			continue
		}
		want[p.Name] = true
		s.mu.Lock()
		_, exists := s.boxes[p.Name]
		s.mu.Unlock()
		if exists {
			s.App.SetUIURL(p.Name, s.boxURL(p.Name))
			continue
		}
		if err := s.openBox(p.Name, root); err != nil {
			continue
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, box := range s.boxes {
		if want[name] {
			continue
		}
		if box.srv != nil {
			_ = box.srv.Close()
		}
		delete(s.boxes, name)
		s.App.SetUIURL(name, "")
	}
}

func (s *Server) boxURL(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.boxes[name]; b != nil {
		return b.url
	}
	return ""
}

func (s *Server) openBox(name, root string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	box := &sandbox{
		app:     s.App,
		name:    name,
		root:    root,
		url:     "http://" + ln.Addr().String() + "/",
		apiBase: s.App.PluginAPIBase(),
		parent:  s.URL(),
	}
	box.srv = &http.Server{Handler: box.handler(), ReadHeaderTimeout: 5 * time.Second}
	s.mu.Lock()
	s.boxes[name] = box
	s.mu.Unlock()
	s.App.SetUIURL(name, box.url)
	go func() { _ = box.srv.Serve(ln) }()
	return nil
}

func (b *sandbox) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"):
			b.proxy(w, r)
		case strings.HasPrefix(r.URL.Path, "/fonts/"):
			b.fonts(w, r)
		case r.URL.Path == "/theme.css":
			b.coreFile(w, r, "theme.css")
		case r.URL.Path == "/theme-boot.js":
			b.coreFile(w, r, "theme-boot.js")
		case r.URL.Path == "/" || r.URL.Path == "/index.html":
			b.serveIndex(w, r)
		default:
			http.FileServer(http.Dir(b.root)).ServeHTTP(w, r)
		}
	})
}

func (b *sandbox) serveIndex(w http.ResponseWriter, r *http.Request) {
	raw, err := os.ReadFile(filepath.Join(b.root, "index.html"))
	if err != nil {
		http.Error(w, "missing plugin ui", http.StatusNotFound)
		return
	}
	mode := chosenTheme(r, b.app.Theme())
	inbox := strings.TrimRight(b.parent, "/") + "/#/inbox"
	boot := `<meta name="dap-token" content="` + b.app.Token(b.name) + `" />` +
		`<meta name="dap-api" content="/api" />` +
		`<meta name="dap-plugin" content="` + b.name + `" />` +
		`<meta name="dap-inbox" content="` + inbox + `" />` +
		`<meta name="dap-theme" content="` + mode + `" />`
	html := injectTheme(bytes.Replace(raw, []byte("<!--DAP_BOOT-->"), []byte(boot), 1), mode)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(html)
}

func (b *sandbox) coreFile(w http.ResponseWriter, r *http.Request, name string) {
	static, err := fs.Sub(web.Static, "static")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	raw, err := fs.ReadFile(static, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(name, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(name, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

func (b *sandbox) fonts(w http.ResponseWriter, r *http.Request) {
	static, err := fs.Sub(web.Static, "static")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.StripPrefix("/fonts/", http.FileServer(http.FS(mustSub(static, "fonts")))).ServeHTTP(w, r)
}

func mustSub(parent fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(parent, dir)
	if err != nil {
		return parent
	}
	return sub
}

func (b *sandbox) proxy(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api")
	target := strings.TrimRight(b.apiBase, "/") + "/v1/" + b.name + rest
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "proxy", http.StatusBadGateway)
		return
	}
	if auth := r.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "plugin API unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
