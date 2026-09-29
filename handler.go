package vanity

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

// Handler answers go-get requests for vanity import paths.
// Create one with [New].
type Handler struct {
	cfg Config
}

// New returns a Handler for cfg. It reports an error if cfg is invalid.
// New copies cfg, so later changes to it have no effect.
func New(cfg Config) (*Handler, error) {
	cfg.Modules = append([]Module(nil), cfg.Modules...)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg.sortModules()
	return &Handler{cfg: cfg}, nil
}

// ServeHTTP answers go-get requests with the go-import and go-source meta
// tags and redirects every other request for a module path to the
// configured Redirect URL. Unknown paths get 404 Not Found.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m, ok := h.cfg.resolve(h.host(r), r.URL.Path)
	switch {
	case ok && isGoGet(r):
		h.serveGoGet(w, m)
	case ok:
		http.Redirect(w, r, m.redirect, http.StatusFound)
	case strings.Trim(r.URL.Path, "/") == "" && h.cfg.Index != "" && !isGoGet(r):
		http.Redirect(w, r, h.cfg.Index, http.StatusFound)
	default:
		http.NotFound(w, r)
	}
}

// Middleware answers go-get requests for vanity import paths and passes
// every other request to next. Use it to add vanity import paths to an
// existing website; browsers are not redirected, so the site keeps serving
// its own pages.
func (h *Handler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isGoGet(r) && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			if m, ok := h.cfg.resolve(h.host(r), r.URL.Path); ok {
				h.serveGoGet(w, m)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) host(r *http.Request) string {
	if h.cfg.Host != "" {
		return h.cfg.Host
	}
	return r.Host
}

func isGoGet(r *http.Request) bool {
	return r.URL.Query().Get("go-get") == "1"
}

type page struct {
	Import string
	Source string
	Link   string
}

var pageTmpl = template.Must(template.New("").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="go-import" content="{{.Import}}">
{{if .Source}}<meta name="go-source" content="{{.Source}}">
{{end}}</head>
<body>
<a href="{{.Link}}">{{.Link}}</a>
</body>
</html>
`))

func (h *Handler) serveGoGet(w http.ResponseWriter, m match) {
	imp := m.module + " " + m.vcs + " " + m.repo
	if m.subdir != "" {
		imp += " " + m.subdir
	}
	p := page{Import: imp, Link: m.redirect}
	if src := goSource(m); src != "" {
		p.Source = m.module + " " + src
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	pageTmpl.Execute(w, p)
}

// goSource returns the home, directory and file templates of the go-source
// meta tag for repositories on well-known hosts, or "" for other hosts.
func goSource(m match) string {
	if m.vcs != "git" {
		return ""
	}
	u, err := url.Parse(m.repo)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	home := strings.TrimSuffix(strings.TrimSuffix(m.repo, "/"), ".git")
	var tree, blob string
	switch u.Host {
	case "github.com":
		tree, blob = home+"/tree/HEAD", home+"/blob/HEAD"
	case "gitlab.com":
		tree, blob = home+"/-/tree/HEAD", home+"/-/blob/HEAD"
	default:
		return ""
	}
	if m.subdir != "" {
		tree += "/" + m.subdir
		blob += "/" + m.subdir
	}
	return home + " " + tree + "{/dir} " + blob + "{/dir}/{file}#L{line}"
}
