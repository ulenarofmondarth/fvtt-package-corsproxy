# Contributing

Thank you for your interest. The developer documentation at <https://ulenarofmondarth.github.io/fvtt-package-corsproxy/> (sources in [`docs/developer/`](docs/developer/)) covers the design and tooling in more depth. This file is the short version.

## Getting set up

You need Go 1.27 or later, [golangci-lint](https://golangci-lint.run) 2.15, Node.js 24 or later with [pnpm](https://pnpm.io) 12, and Docker.

```shell
git clone <URL of your fork>
cd fvtt-package-corsproxy
pnpm install    # the commit hooks and the release tooling
make run        # the proxy on http://localhost:8080
```

The proxy is Go, using only the standard library. Node.js is only for the repository's tooling. The project only accepts pnpm; npm and yarn installs are refused. The documentation is built in Docker, so no Python is installed on your machine.

## Workflow

1. Fork the repository and create a short-lived branch from `dev`, named for the change: `feat/...`, `fix/...`, `docs/...`.
2. Write a failing test first, then the change. A bug fix starts with a test that reproduces the bug.
3. Run the checks (below).
4. Commit using Conventional Commits (see below).
5. Open a pull request against `dev`. The **CI passed** check must be green before it is merged.

Keep pull requests small and focused, and update the documentation alongside the code. A new setting also goes in `.env.example`, `make config` and the configuration page of the documentation.

`dev` is where work is integrated; `main` holds what has been released. Releases are made by the maintainer through a pull request from `dev` into `main`, merged with a merge commit (never squashed or rebased, since the individual commit messages decide the version).

## Checks

| Command              | What it checks                                                      |
| -------------------- | ------------------------------------------------------------------- |
| `pnpm format:check`  | gofmt and Prettier (`pnpm format` fixes both)                       |
| `pnpm lint`          | `go vet`, golangci-lint, and ESLint over the tooling; warnings fail |
| `pnpm typecheck`     | The tooling's TypeScript                                            |
| `pnpm test`          | The Go tests, then the tests of the repository's tooling            |
| `pnpm test:coverage` | The same, with the Go coverage gate                                 |
| `make dist`          | The release binaries cross-compile                                  |
| `make smoke`         | A real server against real sites (needs the network)                |
| `pnpm build:docs`    | The documentation builds with no warnings                           |

CI runs all of these except `make smoke` on every pull request, builds the container image, and checks the commit messages.

## Code standards

- Go code is formatted by gofmt and passes `go vet` and golangci-lint (`.golangci.yml`).
- Stay with the standard library unless a dependency is clearly worth it; say why in the pull request.
- Tests live beside the code in `*_test.go`, and need no network. Tests of the repository's tooling are in `tests/`.
- Keep the defaults safe to run locally, and log a warning at start-up for any setting whose default is unsafe in production.

## Commit messages

Commits must follow [Conventional Commits](https://www.conventionalcommits.org), with these extra rules:

- a scope is required: `fix(proxy): …`;
- the header is at most 72 characters;
- the subject is in the imperative mood: "add", not "added" or "adds".

The type decides the next version number, so pick it with care:

| Type                                                                | Release |
| ------------------------------------------------------------------- | ------- |
| `fix`                                                               | patch   |
| `feat`                                                              | minor   |
| `!` after the scope, or a `BREAKING CHANGE:` footer                 | major   |
| `docs`, `test`, `refactor`, `style`, `perf`, `build`, `ci`, `chore` | none    |

When you commit from a terminal, Commitizen prompts you for the parts of the message (or run `pnpm commit`). commitlint then rejects any message that does not conform. Reference issues with `Closes #N` in the body so that the release notes link to them.

Releases are fully automated: merging `dev` into `main` runs CI and then semantic-release, which versions, tags, attaches the binaries and publishes the container image, and then merges the release commit back into `dev`. Never edit the version number or `CHANGELOG.md` by hand.

## Git hooks

The pre-commit hook runs gofmt, `go vet`, golangci-lint and the Go tests when Go files are staged, and Prettier, ESLint, the type check and the related tests for the tooling. The commit-msg hook runs commitlint.

### Emergency bypass

Do not bypass the hooks as a matter of routine. If you genuinely must, for example to save work when a hook itself is broken:

```shell
git commit --no-verify -m "fix(scope): describe the change"
```

Before you push, fix whatever blocked the hooks, then revalidate:

```shell
pnpm format:check && pnpm lint && pnpm typecheck && pnpm test
git commit --amend --no-edit
```

The first line repeats the pre-commit checks by hand; the amend alone would not, because the hook only looks at staged files. The amend re-runs commitlint on the message. If either fails, fix the problem and repeat until both pass. Say in the pull request that the hooks were bypassed and why. CI runs the same checks regardless, so a bypassed commit that does not pass will not be merged.

## AI Disclosure

Parts of this project, including code, tests, configuration and documentation, were produced with AI assistance. All changes are human-reviewed before they are merged. If you use AI tools for a contribution, review the output yourself before submitting it; you are responsible for what you submit.
