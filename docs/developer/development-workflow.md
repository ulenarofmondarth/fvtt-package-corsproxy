# Development workflow

(project-branches)=

## Branches

- `dev` is where work is integrated, and the default branch.
- `main` holds what has been released. Every push to it is released (see {ref}`release-workflow`).
- Work happens on short-lived branches cut from `dev`, named for the change: `feat/…`, `fix/…`, `docs/…`, `chore/…`. Pull requests go to `dev`.
- A release is a pull request from `dev` to `main`, merged with a **merge commit**, never squashed or rebased: semantic-release reads the individual commit messages to decide the version.

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org), with these extra rules, enforced by commitlint (`commitlint.config.ts`):

- a scope is required, in kebab-case: `feat(cache): …`;
- the header is at most 72 characters;
- the subject is in the imperative mood: "add", not "added", "adds" or "adding".

The type decides the next version:

| Type                                                                | Release |
| ------------------------------------------------------------------- | ------- |
| `fix`                                                               | patch   |
| `feat`                                                              | minor   |
| `!` after the scope, or a `BREAKING CHANGE:` footer                 | major   |
| `docs`, `test`, `refactor`, `style`, `perf`, `build`, `ci`, `chore` | none    |

Committing from a terminal starts Commitizen's prompt (or run `pnpm commit`). Reference issues with `Closes #N` in the body, so the release notes link them.

Never edit the version in `package.json` or `CHANGELOG.md` by hand; the release does both.

## Git hooks

Husky installs them on `pnpm install`.

- **pre-commit** runs lint-staged on the staged files (`.lintstagedrc.json`): for Go files, `gofmt -w`, `go vet`, `golangci-lint` and `go test`; for the tooling's TypeScript, ESLint, Prettier, the type check and the related tests; Prettier for JSON, Markdown and YAML.
- **prepare-commit-msg** starts Commitizen when you commit from a terminal without a message.
- **commit-msg** runs commitlint.

(emergency-bypass)=

### Emergency bypass

Do not bypass the hooks as a matter of routine. If you genuinely must, for example to save work when a hook itself is broken:

```shell
git commit --no-verify -m "fix(proxy): describe the change"
```

Before you push, fix whatever blocked the hooks, then revalidate:

```shell
pnpm format:check && pnpm lint && pnpm typecheck && pnpm test
git commit --amend --no-edit
```

The first line repeats the pre-commit checks by hand; the amend alone would not, because the hook only looks at staged files. The amend re-runs commitlint on the message. Say in the pull request that the hooks were bypassed, and why. CI runs the same checks regardless.

## Checks

| Command              | What it checks                                                          |
| -------------------- | ----------------------------------------------------------------------- |
| `pnpm format:check`  | gofmt and Prettier (`pnpm format` fixes both)                           |
| `pnpm lint`          | `go vet`, golangci-lint, and ESLint over the tooling; warnings fail     |
| `pnpm typecheck`     | The tooling's TypeScript, with TypeScript 7 (`pnpm typecheck:fallback` uses TypeScript 6) |
| `pnpm test`          | The Go tests, then the tooling's tests                                  |
| `pnpm test:coverage` | The Go tests with the coverage gate, then the tooling's tests           |
| `make smoke`         | A real server against real sites (needs the network; not run in CI)    |
| `make dist`          | The release binaries cross-compile                                      |
| `pnpm build:docs`    | The documentation builds with no warnings                               |
