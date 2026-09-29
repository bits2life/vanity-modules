package vanity

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Environment variables read by [FromEnv].
const (
	EnvConfig   = "VANITY_CONFIG"   // path to a JSON config file
	EnvHost     = "VANITY_HOST"     // Config.Host
	EnvRepo     = "VANITY_REPO"     // Config.Repo
	EnvVCS      = "VANITY_VCS"      // Config.VCS
	EnvRedirect = "VANITY_REDIRECT" // Config.Redirect
	EnvIndex    = "VANITY_INDEX"    // Config.Index
)

// Parse reads a JSON config from r. Unknown fields are an error, so typos
// are caught early.
func Parse(r io.Reader) (Config, error) {
	var c Config
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("vanity: parse config: %w", err)
	}
	return c, nil
}

// LoadFile reads a JSON config from the named file.
func LoadFile(name string) (Config, error) {
	f, err := os.Open(name)
	if err != nil {
		return Config{}, fmt.Errorf("vanity: %w", err)
	}
	defer f.Close()
	c, err := Parse(f)
	if err != nil {
		return Config{}, fmt.Errorf("%w (in %s)", err, name)
	}
	return c, nil
}

// FromEnv builds a Config from the environment. If VANITY_CONFIG names a
// file, it is loaded first; VANITY_HOST, VANITY_REPO, VANITY_VCS,
// VANITY_REDIRECT and VANITY_INDEX then override the matching fields when
// set.
func FromEnv() (Config, error) {
	var c Config
	if name := os.Getenv(EnvConfig); name != "" {
		var err error
		if c, err = LoadFile(name); err != nil {
			return Config{}, err
		}
	}
	for env, field := range map[string]*string{
		EnvHost:     &c.Host,
		EnvRepo:     &c.Repo,
		EnvVCS:      &c.VCS,
		EnvRedirect: &c.Redirect,
		EnvIndex:    &c.Index,
	} {
		if v, ok := os.LookupEnv(env); ok && v != "" {
			*field = v
		}
	}
	return c, nil
}
