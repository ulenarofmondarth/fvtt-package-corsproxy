# Setting up

The proxy is Go, using only the standard library. The repository's tooling (commit hooks, semantic-release and the tests of their configuration) runs on Node.js, and the documentation is built in Docker.

## Prerequisites

1. **git**, from <https://git-scm.com/> or a package manager.
2. **Go 1.27 or later**, from <https://go.dev/dl/>. The version CI uses is the one in `go.mod`.
3. **golangci-lint 2.15**, which the pre-commit hook and `make lint` run:

   ```shell
   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
   ```

   Make sure `$(go env GOPATH)/bin` is on your `PATH`.
4. **Node.js 24 or later** and [pnpm](https://pnpm.io/installation) 12. The project only accepts pnpm; npm and yarn are refused.
5. **Docker**, for the documentation build and the container image. No Python is installed on the host: the Sphinx toolchain lives in `docs/Dockerfile`.

## Getting the code

```shell
git clone https://github.com/ulenarofmondarth/fvtt-package-corsproxy.git
cd fvtt-package-corsproxy
pnpm install      # also installs the git hooks
make run          # the proxy on http://localhost:8080
```

`make dev` runs it with hot reload ([air](https://github.com/air-verse/air), installed on first use). `make help` lists every target.

## The documentation

```shell
pnpm build:docs       # docs/_build/html; any warning fails the build
pnpm devserve:docs    # live preview on http://localhost:8088
```

## The tooling

The tooling is plain JavaScript (ES modules), with types in JSDoc comments where they help: the commitlint and semantic-release configuration, and the tests in `tests/`. There is no TypeScript and no compiler to keep up to date.
