package vanity

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newHandler(t *testing.T, cfg Config) *Handler {
	t.Helper()
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(h http.Handler, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

var testConfig = Config{
	Host: "bits2life.com",
	Repo: "https://github.com/bits2life/{name}",
	Modules: []Module{
		{Path: "tools", Repo: "https://gitlab.com/bits2life/tools"},
		{Path: "tools/lint", Repo: "https://github.com/bits2life/lint"},
		{Path: "bits2life.com/mono/foo", Repo: "https://github.com/bits2life/mono", Subdir: "/foo/"},
		{Path: "hg", VCS: "hg", Repo: "https://hg.example.com/hg", Redirect: "https://docs.example.com/{import}"},
	},
}

func TestGoGet(t *testing.T) {
	h := newHandler(t, testConfig)
	tests := []struct {
		path, importMeta, sourceMeta string
	}{
		{"/foo", "bits2life.com/foo git https://github.com/bits2life/foo",
			"bits2life.com/foo https://github.com/bits2life/foo https://github.com/bits2life/foo/tree/HEAD{/dir} https://github.com/bits2life/foo/blob/HEAD{/dir}/{file}#L{line}"},
		{"/foo/bar/baz", "bits2life.com/foo git https://github.com/bits2life/foo", ""},
		{"/tools", "bits2life.com/tools git https://gitlab.com/bits2life/tools",
			"bits2life.com/tools https://gitlab.com/bits2life/tools https://gitlab.com/bits2life/tools/-/tree/HEAD{/dir} https://gitlab.com/bits2life/tools/-/blob/HEAD{/dir}/{file}#L{line}"},
		{"/tools/other", "bits2life.com/tools git https://gitlab.com/bits2life/tools", ""},
		{"/tools/lint/cmd", "bits2life.com/tools/lint git https://github.com/bits2life/lint", ""},
		{"/tools/linter", "bits2life.com/tools git https://gitlab.com/bits2life/tools", ""},
		{"/mono/foo/x", "bits2life.com/mono/foo git https://github.com/bits2life/mono foo",
			"bits2life.com/mono/foo https://github.com/bits2life/mono https://github.com/bits2life/mono/tree/HEAD/foo{/dir} https://github.com/bits2life/mono/blob/HEAD/foo{/dir}/{file}#L{line}"},
		{"/hg", "bits2life.com/hg hg https://hg.example.com/hg", ""},
	}
	for _, tt := range tests {
		w := get(h, tt.path+"?go-get=1")
		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d", tt.path, w.Code)
			continue
		}
		body := w.Body.String()
		want := `<meta name="go-import" content="` + tt.importMeta + `">`
		if !strings.Contains(body, want) {
			t.Errorf("%s: missing %s in\n%s", tt.path, want, body)
		}
		if tt.sourceMeta != "" {
			want := `<meta name="go-source" content="` + tt.sourceMeta + `">`
			if !strings.Contains(body, want) {
				t.Errorf("%s: missing %s in\n%s", tt.path, want, body)
			}
		}
	}
}

func TestNoGoSourceForOtherHosts(t *testing.T) {
	h := newHandler(t, testConfig)
	if body := get(h, "/hg?go-get=1").Body.String(); strings.Contains(body, "go-source") {
		t.Errorf("unexpected go-source:\n%s", body)
	}
}

func TestRedirect(t *testing.T) {
	cfg := testConfig
	h := newHandler(t, cfg)
	tests := map[string]string{
		"/foo":        "https://github.com/bits2life/foo",
		"/foo/bar":    "https://github.com/bits2life/foo",
		"/tools/lint": "https://github.com/bits2life/lint",
		"/hg/sub":     "https://docs.example.com/bits2life.com/hg/sub",
	}
	for path, want := range tests {
		w := get(h, path)
		if w.Code != http.StatusFound || w.Header().Get("Location") != want {
			t.Errorf("%s: got %d %q, want 302 %q", path, w.Code, w.Header().Get("Location"), want)
		}
	}

	cfg.Redirect = "https://pkg.go.dev/{import}"
	h = newHandler(t, cfg)
	if got := get(h, "/foo/bar").Header().Get("Location"); got != "https://pkg.go.dev/bits2life.com/foo/bar" {
		t.Errorf("pkg.go.dev redirect: got %q", got)
	}
}

func TestIndexAndNotFound(t *testing.T) {
	h := newHandler(t, testConfig)
	if w := get(h, "/"); w.Code != http.StatusNotFound {
		t.Errorf("root without index: got %d", w.Code)
	}

	cfg := testConfig
	cfg.Index = "https://github.com/bits2life"
	h = newHandler(t, cfg)
	if w := get(h, "/"); w.Code != http.StatusFound || w.Header().Get("Location") != cfg.Index {
		t.Errorf("root with index: got %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := get(h, "/?go-get=1"); w.Code != http.StatusNotFound {
		t.Errorf("root go-get: got %d", w.Code)
	}

	for _, path := range []string{"/.hidden?go-get=1", "/a/../b?go-get=1", "/a//b?go-get=1"} {
		if w := get(h, path); w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d", path, w.Code)
		}
	}
}

func TestModulesOnly(t *testing.T) {
	cfg := testConfig
	cfg.Repo = ""
	h := newHandler(t, cfg)
	if w := get(h, "/foo?go-get=1"); w.Code != http.StatusNotFound {
		t.Errorf("unlisted module: got %d", w.Code)
	}
	if w := get(h, "/tools?go-get=1"); w.Code != http.StatusOK {
		t.Errorf("listed module: got %d", w.Code)
	}
}

func TestHostFromRequest(t *testing.T) {
	h := newHandler(t, Config{Repo: "https://github.com/bits2life/{name}"})
	r := httptest.NewRequest(http.MethodGet, "/foo?go-get=1", nil)
	r.Host = "go.example.org"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), `content="go.example.org/foo git `) {
		t.Errorf("host not taken from request:\n%s", w.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newHandler(t, testConfig)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/foo?go-get=1", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got %d", w.Code)
	}
}

func TestMiddleware(t *testing.T) {
	h := newHandler(t, testConfig)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("site"))
	})
	mw := h.Middleware(next)

	if body := get(mw, "/foo?go-get=1").Body.String(); !strings.Contains(body, "go-import") {
		t.Errorf("go-get not answered:\n%s", body)
	}
	for _, path := range []string{"/foo", "/?go-get=1", "/about"} {
		if body := get(mw, path).Body.String(); body != "site" {
			t.Errorf("%s: not passed through, got %q", path, body)
		}
	}
}

func TestValidate(t *testing.T) {
	bad := []Config{
		{Repo: "https://github.com/bits2life/fixed"},
		{Modules: []Module{{Repo: "https://github.com/x/y"}}},
		{Modules: []Module{{Path: "a"}}},
		{Modules: []Module{{Path: "a/../b", Repo: "https://github.com/x/y"}}},
		{Modules: []Module{{Path: "a", Repo: "r"}, {Path: "/a/", Repo: "r"}}},
	}
	for i, cfg := range bad {
		if _, err := New(cfg); err == nil {
			t.Errorf("config %d: expected error", i)
		}
	}
}

func TestNewCopiesModules(t *testing.T) {
	mods := []Module{{Path: "a", Repo: "https://github.com/x/a"}, {Path: "a/b", Repo: "https://github.com/x/b"}}
	newHandler(t, Config{Modules: mods})
	if mods[0].Path != "a" {
		t.Errorf("caller's modules were reordered: %v", mods)
	}
}

func TestParse(t *testing.T) {
	c, err := Parse(strings.NewReader(`{
		"host": "bits2life.com",
		"repo": "https://github.com/bits2life/{name}",
		"modules": [{"path": "x/y", "repo": "https://github.com/bits2life/xy", "subdir": "y"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "bits2life.com" || len(c.Modules) != 1 || c.Modules[0].Subdir != "y" {
		t.Errorf("unexpected config: %+v", c)
	}
	if _, err := Parse(strings.NewReader(`{"hots": "typo"}`)); err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestFromEnv(t *testing.T) {
	file := filepath.Join(t.TempDir(), "vanity.json")
	json := `{"host": "file.example", "vcs": "git", "modules": [{"path": "m", "repo": "https://github.com/x/m"}]}`
	if err := os.WriteFile(file, []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfig, file)
	t.Setenv(EnvHost, "bits2life.com")
	t.Setenv(EnvRepo, "https://github.com/bits2life/{name}")
	t.Setenv(EnvRedirect, "https://pkg.go.dev/{import}")
	t.Setenv(EnvIndex, "")

	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "bits2life.com" || c.Repo != "https://github.com/bits2life/{name}" ||
		c.Redirect != "https://pkg.go.dev/{import}" || c.VCS != "git" || len(c.Modules) != 1 {
		t.Errorf("unexpected config: %+v", c)
	}

	t.Setenv(EnvConfig, filepath.Join(t.TempDir(), "missing.json"))
	if _, err := FromEnv(); err == nil {
		t.Error("expected error for missing file")
	}
}
