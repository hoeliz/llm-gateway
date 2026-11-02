# Continuous integration

`workflows/ci.yml` runs on every push and pull request using Ubuntu 24.04,
the Go version declared in `go.mod` (currently 1.22), and Node.js 24.

The workflow installs the locked frontend dependencies with `npm ci`, runs
the JavaScript tests, builds the React/Vite interface, runs `go test -race ./...`
and `go vet ./...`, and compiles `./cmd/gateway`. The frontend build must come
before the Go checks because `internal/webui/dist` is embedded in the executable.

CI fails if rebuilding the frontend changes, removes, or creates files in
`internal/webui/dist`. After modifying the interface or its dependencies, run
`make ui` and commit the complete generated directory along with the sources.
The check only reads Git status; CI never commits generated files.

To reproduce the checks locally with Go 1.22, a C compiler (for the race
detector), and Node.js 24:

```sh
make ui
make test
make build
git status --porcelain --untracked-files=all -- internal/webui/dist
```

The last command must produce no output. The npm cache is keyed by
`web/package-lock.json`; `node_modules` and compiled frontend assets are not
cached. Go's module/build cache is keyed by `go.mod` and `go.sum` when present
(the project currently uses only the standard library and has no `go.sum`).

Actions are pinned to commit SHAs. The token has only `contents: read`, checkout
does not persist credentials, and no provider API keys or other secrets are
needed. Pull requests use `pull_request`, including forks. Each job has a
15-minute timeout; superseded runs for the same event/ref are cancelled.
