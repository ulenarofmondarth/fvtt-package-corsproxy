# Testing

## The proxy

The Go tests are in `*_test.go` beside the code (`main_test.go`, `security_test.go`). They run the handlers against `httptest` servers, so they need no network.

```shell
make test        # go test ./...
make coverage    # with the coverage gate
```

The coverage gate is `COVERAGE_MIN` in the `Makefile`, a percentage of statements. Ratchet it up as coverage improves; never lower it to get a change through. CI writes the per-function coverage to the job's summary and uploads it to Codecov.

Every bug fix starts with a test that fails because of the bug.

## Smoke test

`make smoke` runs `test.sh`: it starts a real server and sends requests through it to real sites (GitHub's API, Project Gutenberg, httpbin). It needs the network, and those sites, so it is not run in CI. Run it by hand before a release that changes how requests are sent.

## Benchmarks

`bench/` holds the harness behind upstream's performance comparison: a local upstream server, the proxy's image and a probe script. See `bench/README.md`.

## The tooling

`tests/config/` checks the repository's configuration with Vitest, so mistakes show up in CI rather than at release time:

- `commitlint.test.js`: the commit message rules;
- `release.test.js`: the semantic-release configuration, the version bump (pnpm, offline, nothing but the version changes) and the release workflow's wiring;
- `make.test.js`: the Makefile's coverage gate;
- `scripts.test.js`: the package scripts run only tools the project installs, not global ones, and the tooling stays free of TypeScript;
- `ci.test.js`: the Codecov upload fails CI when a token is set, and only then;
- `action-pins.test.js`: third-party actions are pinned to a commit (see {doc}`ci-cd`).

```shell
pnpm test:repo
```
