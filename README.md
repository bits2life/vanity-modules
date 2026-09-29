# vanity-modules

`bits2life.com/vanity-modules` serves Go vanity import paths. It lets
`go get bits2life.com/foo` fetch a module that lives at
`github.com/bits2life/foo`, and sends people who open the same URL in a
browser to the repository (or to its documentation).

Standard library only.

```sh
go get bits2life.com/vanity-modules
```

## How it works

When the go command resolves `bits2life.com/foo/bar` it requests
`https://bits2life.com/foo/bar?go-get=1` and looks for a `go-import` meta tag.
This package answers with:

```html
<meta name="go-import" content="bits2life.com/foo git https://github.com/bits2life/foo">
<meta name="go-source" content="bits2life.com/foo https://github.com/bits2life/foo ...">
```

Subpackage paths resolve to the module root. A `go-source` tag is added for
GitHub and GitLab repositories.

## Usage

### Pattern mapping in code

```go
h, err := vanity.New(vanity.Config{
	Host: "bits2life.com",
	Repo: "https://github.com/bits2life/{name}",
})
if err != nil {
	log.Fatal(err)
}
http.ListenAndServe(":8080", h)
```

`h` answers go-get requests and redirects browsers to the repository. Paths
that don't match return 404.

### Middleware for an existing site

```go
http.ListenAndServe(":8080", h.Middleware(mySite))
```

The middleware answers only `?go-get=1` requests for module paths and passes
everything else, including browser visits, to `mySite`.

### Configuration from the environment

```go
cfg, err := vanity.FromEnv()
```

| Variable          | Field      | Example                                 |
|-------------------|------------|-----------------------------------------|
| `VANITY_CONFIG`   | JSON file  | `/etc/vanity.json`                      |
| `VANITY_HOST`     | `Host`     | `bits2life.com`                         |
| `VANITY_REPO`     | `Repo`     | `https://github.com/bits2life/{name}`   |
| `VANITY_VCS`      | `VCS`      | `git` (default)                         |
| `VANITY_REDIRECT` | `Redirect` | `https://pkg.go.dev/{import}`           |
| `VANITY_INDEX`    | `Index`    | `https://github.com/bits2life`          |

The JSON file is loaded first; the other variables override it when set.

### Nonstandard mappings in JSON

Modules that don't follow the pattern (nested paths, other names, other
hosts, monorepo subdirectories) go in a JSON file. Listed modules win over
the pattern, and the longest matching path wins. See
[`vanity.example.json`](vanity.example.json):

```json
{
  "host": "bits2life.com",
  "repo": "https://github.com/bits2life/{name}",
  "index": "https://github.com/bits2life",
  "modules": [
    {"path": "tools/lint", "repo": "https://github.com/bits2life/lint-tools"},
    {"path": "mono/foo", "repo": "https://github.com/bits2life/mono", "subdir": "foo"},
    {"path": "legacy", "repo": "https://gitlab.com/bits2life/legacy", "redirect": "https://pkg.go.dev/{import}"}
  ]
}
```

Load it with `vanity.LoadFile("vanity.json")` or through `VANITY_CONFIG`.
Unknown fields are rejected so typos surface at startup. `subdir` needs
Go 1.25 or later on the client.

Omit `repo` to serve only the listed modules.

### Placeholders

| Placeholder | Meaning                          | Example                   |
|-------------|----------------------------------|---------------------------|
| `{name}`    | first path element after host    | `foo`                     |
| `{module}`  | module path                      | `bits2life.com/foo`       |
| `{import}`  | requested import path            | `bits2life.com/foo/bar`   |
| `{repo}`    | resolved repository (redirects)  | `https://github.com/...`  |

## Standalone server

```sh
go install bits2life.com/vanity-modules/cmd/vanity@latest
VANITY_HOST=bits2life.com VANITY_REPO='https://github.com/bits2life/{name}' vanity
```

It listens on `VANITY_ADDR`, else `:$PORT`, else `:8080`. Put it behind
HTTPS; the go command only uses plain HTTP for hosts in `GOINSECURE`.

## License

MIT
