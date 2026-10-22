# CI/CD

Workflows live in `.github/workflows`. Shared steps are composite actions in `.github/actions`: `setup-go` (Go, at the version in `go.mod`), `setup` (pnpm, Node.js and the tooling's dependencies) and `build-docs` (the documentation, in Docker).

(ci-workflow)=

## Dev CI (`dev.yml`)

Runs on every push to `dev`, on every pull request to `dev` or `main`, and as the first stage of a release. Its jobs run in parallel:

| Job             | Checks                                                                                                         |
| --------------- | -------------------------------------------------------------------------------------------------------------- |
| Commit messages | commitlint over the pull request's commits (pull requests only)                                                |
| Lint            | gofmt, `go vet`, golangci-lint, ESLint and the type check over the tooling, Prettier                           |
| Tests           | the Go tests with the coverage gate (summary on the job page, uploaded to Codecov), then the tooling's tests   |
| Build           | cross-compiles the release binaries (kept as an artifact); builds the container image and checks that it starts and reports its version |
| Docs            | calls the Docs workflow                                                                                        |
| CI passed       | succeeds only if all of the above passed; the required status check                                            |

Superseded runs of the same pull request are cancelled.

## Docs (`docs.yml`)

Builds the Sphinx image (cached) and the site, and keeps the HTML as an artifact. Any Sphinx warning fails the build. Called by Dev CI; can also be started by hand.

(release-workflow)=

## Release (`release.yml`)

Runs on every push to `main`:

1. **CI**: the whole of Dev CI.
2. **Semantic release** (`pnpm release`, configured in `release.config.js`):
   1. works out the next version from the commits since the last `v<version>` tag, and stops if none warrants a release;
   2. writes the release notes and `CHANGELOG.md`;
   3. sets the version in `package.json` (with `pnpm pkg set`), then cross-compiles the binaries with it stamped in (`make dist VERSION=v<version>`);
   4. commits `chore(release): v<version> [skip ci]` to `main` and tags it;
   5. creates the GitHub release, with the binaries and `checksums.txt` attached.
3. **Publish the container image**, if anything was released: builds the image from the tag for linux/amd64 and linux/arm64, and pushes it to `ghcr.io/ulenarofmondarth/fvtt-package-corsproxy`, tagged `<version>`, `<major>.<minor>`, `<major>` and `latest`.
4. **Bring dev up to date**, if anything was released: fast-forwards `dev` to the release commit, or opens a pull request from `main` to `dev` if it cannot.
5. **Pages**: builds the documentation for the released tag (or for `main`, if nothing was released) and deploys it to GitHub Pages.

It can also be started by hand (**Release** > **Run workflow**, on `main`). That releases only if there are releasable commits, and otherwise just publishes the documentation again.

The jobs that push to the protected branches authenticate as the release App (see {ref}`release-app`). `@semantic-release/npm` is not used: nothing is published to npm, and it runs `npm version`, which refuses to run in a project that requires pnpm.

:::{tip}
`pnpm release:dry-run` checks the semantic-release configuration locally without releasing anything.
:::

## Other workflows

- **CodeQL** (`codeql.yml`): code scanning of the Go, the tooling's TypeScript and the workflows themselves, on pushes, pull requests and weekly.
- **Dependency review** (`dependency-review.yml`): fails pull requests that add dependencies with known vulnerabilities.
- **Dependabot** (`.github/dependabot.yml`): weekly updates of the Go modules, the base images, the tooling's npm packages (grouped), the documentation's Python requirements and base image, and the GitHub Actions, raised against `dev`.
- **Third-party actions** are pinned to a full commit SHA with the release in a comment (`uses: docker/build-push-action@<sha> # v7.4.0`), because a tag can be moved by whoever controls the action's repository. Dependabot updates the SHA and the comment together. GitHub's own `actions/*` and `github/*` actions use version tags. `tests/config/action-pins.test.ts` enforces this; to add an action, look up the commit of the release (`gh api repos/<owner>/<repo>/commits/<tag> --jq .sha`).
