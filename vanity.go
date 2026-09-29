// Package vanity serves Go vanity import paths.
//
// A vanity import path such as bits2life.com/foo lets users "go get" a module
// that is hosted elsewhere, for example on GitHub. When the go command fetches
// a vanity path it requests https://bits2life.com/foo?go-get=1 and expects an
// HTML page with a go-import meta tag that names the real repository. This
// package answers those requests, and can redirect browsers visiting the same
// URL to the repository or to its documentation.
//
// Most setups need a single pattern: every first path element maps to a
// repository with the same name.
//
//	h, err := vanity.New(vanity.Config{
//		Host: "bits2life.com",
//		Repo: "https://github.com/bits2life/{name}",
//	})
//
// Modules that do not follow the pattern are listed explicitly, either in code
// or in a JSON file loaded with [LoadFile] or [FromEnv].
package vanity

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Config describes how vanity import paths map to repositories.
//
// String fields that act as templates may use these placeholders:
//
//	{name}   the first path element after the host, e.g. "foo"
//	{module} the full module path, e.g. "bits2life.com/foo"
//	{import} the full import path requested, e.g. "bits2life.com/foo/bar"
//	{repo}   the resolved repository URL (Redirect only)
type Config struct {
	// Host is the vanity host, e.g. "bits2life.com". If empty, the Host
	// header of each request is used.
	Host string `json:"host,omitempty"`

	// Repo is the repository URL template used for any single-element path
	// not matched by Modules, e.g. "https://github.com/bits2life/{name}".
	// If empty, only paths listed in Modules are served.
	Repo string `json:"repo,omitempty"`

	// VCS is the version control system of pattern-matched repositories.
	// It defaults to "git".
	VCS string `json:"vcs,omitempty"`

	// Redirect is the URL template browsers are redirected to when they
	// visit a module path. It defaults to "{repo}". Use
	// "https://pkg.go.dev/{import}" to send visitors to the documentation.
	Redirect string `json:"redirect,omitempty"`

	// Index is where browsers visiting the root path are redirected. If
	// empty, the root path returns 404 Not Found.
	Index string `json:"index,omitempty"`

	// Modules lists modules that do not follow the Repo pattern. They take
	// precedence over the pattern, and the longest matching path wins.
	Modules []Module `json:"modules,omitempty"`
}

// Module maps one vanity module path to a repository.
type Module struct {
	// Path is the module path below the host, e.g. "tools/lint" for
	// bits2life.com/tools/lint. A leading host is accepted and removed.
	Path string `json:"path"`

	// Repo is the repository URL, e.g. "https://github.com/bits2life/lint".
	Repo string `json:"repo"`

	// VCS overrides Config.VCS for this module.
	VCS string `json:"vcs,omitempty"`

	// Subdir is the module's directory inside the repository, for modules
	// that do not live at the repository root. It requires Go 1.25 or later
	// on the client.
	Subdir string `json:"subdir,omitempty"`

	// Redirect overrides Config.Redirect for this module.
	Redirect string `json:"redirect,omitempty"`
}

// match is the result of resolving an import path.
type match struct {
	module   string // full module path, e.g. "bits2life.com/foo"
	importP  string // full requested import path
	name     string // first path element
	vcs      string
	repo     string
	subdir   string
	redirect string
}

var validElem = regexp.MustCompile(`^[A-Za-z0-9_~-][A-Za-z0-9._~-]*$`)

func (c *Config) validate() error {
	var errs []error
	if c.Repo != "" && !strings.Contains(c.Repo, "{name}") {
		errs = append(errs, fmt.Errorf("repo %q must contain {name}", c.Repo))
	}
	seen := map[string]bool{}
	for i := range c.Modules {
		m := &c.Modules[i]
		m.Path = cleanModulePath(c.Host, m.Path)
		switch {
		case m.Path == "":
			errs = append(errs, fmt.Errorf("modules[%d]: path is required", i))
			continue
		case !validPath(m.Path):
			errs = append(errs, fmt.Errorf("modules[%d]: invalid path %q", i, m.Path))
		case seen[m.Path]:
			errs = append(errs, fmt.Errorf("modules[%d]: duplicate path %q", i, m.Path))
		}
		seen[m.Path] = true
		if m.Repo == "" {
			errs = append(errs, fmt.Errorf("modules[%d] (%s): repo is required", i, m.Path))
		} else if _, err := url.Parse(m.Repo); err != nil {
			errs = append(errs, fmt.Errorf("modules[%d] (%s): %v", i, m.Path, err))
		}
		m.Subdir = strings.Trim(m.Subdir, "/")
	}
	return errors.Join(errs...)
}

func cleanModulePath(host, p string) string {
	p = strings.Trim(p, "/")
	if host != "" {
		p = strings.TrimPrefix(p, host+"/")
	}
	return p
}

func validPath(p string) bool {
	for _, elem := range strings.Split(p, "/") {
		if !validElem.MatchString(elem) {
			return false
		}
	}
	return true
}

// resolve finds the module serving path, which is the URL path without its
// leading slash. host is the vanity host for this request.
func (c *Config) resolve(host, path string) (match, bool) {
	path = strings.Trim(path, "/")
	if path == "" || !validPath(path) {
		return match{}, false
	}
	importPath := host + "/" + path

	// Modules is sorted longest path first, so the first hit is the most
	// specific one.
	for _, m := range c.Modules {
		if path == m.Path || strings.HasPrefix(path, m.Path+"/") {
			vcs := firstNonEmpty(m.VCS, c.VCS, "git")
			name, _, _ := strings.Cut(m.Path, "/")
			return c.expand(match{
				module:   host + "/" + m.Path,
				importP:  importPath,
				name:     name,
				vcs:      vcs,
				repo:     m.Repo,
				subdir:   m.Subdir,
				redirect: firstNonEmpty(m.Redirect, c.Redirect, "{repo}"),
			}), true
		}
	}

	if c.Repo == "" {
		return match{}, false
	}
	name, _, _ := strings.Cut(path, "/")
	return c.expand(match{
		module:   host + "/" + name,
		importP:  importPath,
		name:     name,
		vcs:      firstNonEmpty(c.VCS, "git"),
		repo:     c.Repo,
		redirect: firstNonEmpty(c.Redirect, "{repo}"),
	}), true
}

// expand fills the placeholders in m.repo and m.redirect.
func (c *Config) expand(m match) match {
	r := strings.NewReplacer("{name}", m.name, "{module}", m.module, "{import}", m.importP)
	m.repo = r.Replace(m.repo)
	m.redirect = strings.ReplaceAll(r.Replace(m.redirect), "{repo}", m.repo)
	return m
}

func (c *Config) sortModules() {
	sort.SliceStable(c.Modules, func(i, j int) bool {
		return len(c.Modules[i].Path) > len(c.Modules[j].Path)
	})
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
