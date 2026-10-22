# GitHub setup

The repository is <https://github.com/ulenarofmondarth/fvtt-package-corsproxy>, a fork of [melihbirim/corsproxy](https://github.com/melihbirim/corsproxy). This page describes what it needs on GitHub for the workflows in {doc}`ci-cd` to work.

## Branches

- `dev` is the default branch, so that pull requests target it. See {ref}`project-branches`.
- Each branch is protected by a ruleset (Settings → Rules → Rulesets): "Protect dev" and "Protect main". Both:
  - **require a pull request before merging**, with **0** required approvals, merged with a **merge commit** only, so that semantic-release sees every commit and the branches do not diverge;
  - **require the status check** **CI passed** (from GitHub Actions);
  - **block force pushes and deletion**;
  - have one entry on the **bypass list**: the release App (below), mode **Always**.

:::{important}
The required check is matched by name. If the `ci-passed` job in `dev.yml` is renamed, update both rulesets, or pull requests will wait for a check that never reports.
:::

(release-app)=

## The release App

The release workflow pushes to the protected branches: semantic-release pushes the release commit and tag to `main`, and the _Bring dev up to date_ job fast-forwards `dev`. It does this as a GitHub App, the only actor allowed to bypass the rulesets. Each run gets a token that expires within an hour.

To set it up:

1. **Create the App**: account Settings → Developer settings → GitHub Apps → **New GitHub App**.
   - Name: anything unique, for example `ulenarofmondarth-corsproxy-release`. Homepage URL: the repository's URL.
   - Webhook: untick **Active**.
   - Repository permissions: **Contents**, **Pull requests** and **Issues**, all **Read and write**.
   - Where can this App be installed: **Only on this account**.
2. **Generate a private key** on the App's page, and note its **Client ID** (it starts with `Iv`).
3. **Install the App** on this repository only.
4. **Store the credentials** in the repository (Settings → Secrets and variables → Actions): the Client ID as the _variable_ `RELEASE_APP_CLIENT_ID`, and the whole `.pem` file as the _secret_ `RELEASE_APP_PRIVATE_KEY`. Then delete the downloaded file.
5. **Add the App to the rulesets' bypass list**.

An App made for another repository can be installed on this one too, instead of creating a new one.

## Secrets and variables

| Name                      | Kind     | Used by | Purpose                                                                       |
| ------------------------- | -------- | ------- | ----------------------------------------------------------------------------- |
| `RELEASE_APP_CLIENT_ID`   | Variable | Release | The release App's Client ID                                                   |
| `RELEASE_APP_PRIVATE_KEY` | Secret   | Release | The release App's private key                                                 |
| `CODECOV_TOKEN`           | Secret   | Dev CI  | Uploads coverage to [Codecov](https://about.codecov.io). Optional: CI passes without it |

`GITHUB_TOKEN` covers everything else, including pushing the container image to ghcr.io.

## Settings

- **Issues**: a fork has them off by default. Turn them on (Settings → General → Features) for the issue forms in `.github/ISSUE_TEMPLATE`.
- **Pull requests**: allow merge commits. The rulesets decide which merge methods each branch accepts.
- **Pages**: Settings → Pages → Source: **GitHub Actions**. Turning Pages on creates the `github-pages` environment, which only lets the default branch, `dev`, deploy. The release workflow deploys from `main`, so change it (Settings → Environments → `github-pages` → Deployment branches and tags): add `main`, and remove `dev`, so that only released documentation is published. Otherwise the release's Pages job fails with `Branch "main" is not allowed to deploy to github-pages due to environment protection rules`.
- **Packages**: the release pushes the image to `ghcr.io/ulenarofmondarth/fvtt-package-corsproxy`, linked to this repository, and it takes the repository's visibility: public, so it can be pulled without logging in. Nothing to set.
- **Code security**: enable Dependabot alerts and security updates, code scanning, and private vulnerability reporting (which `SECURITY.md` points to).

## Release tags

semantic-release finds the previous version from the `v<version>` tags. The fork carries on from upstream's 1.2.6: `v1.2.6` marks it, on the last upstream commit. Its first release is `1.2.7` or `1.3.0`.
