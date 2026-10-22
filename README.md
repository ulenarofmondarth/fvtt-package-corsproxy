# Foundry VTT Package CORS Proxy

[![Dev CI](https://github.com/ulenarofmondarth/fvtt-package-corsproxy/actions/workflows/dev.yml/badge.svg?branch=dev)](https://github.com/ulenarofmondarth/fvtt-package-corsproxy/actions/workflows/dev.yml)
[![Release](https://github.com/ulenarofmondarth/fvtt-package-corsproxy/actions/workflows/release.yml/badge.svg)](https://github.com/ulenarofmondarth/fvtt-package-corsproxy/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/ulenarofmondarth/fvtt-package-corsproxy/branch/dev/graph/badge.svg)](https://codecov.io/gh/ulenarofmondarth/fvtt-package-corsproxy)
[![Latest release](https://img.shields.io/github/v/release/ulenarofmondarth/fvtt-package-corsproxy)](https://github.com/ulenarofmondarth/fvtt-package-corsproxy/releases/latest)
[![Licence](https://img.shields.io/github/license/ulenarofmondarth/fvtt-package-corsproxy)](LICENSE)

A small, fast CORS proxy written in Go, for looking up Foundry VTT packages from the browser.

Foundry's package listing, GitHub's release downloads and many manifest hosts send no CORS headers, so a module running in Foundry, such as [EMMA](https://github.com/Mondarth/EMMA), cannot read them directly. This proxy fetches them on the module's behalf and adds the headers.

It is a fork of [melihbirim/corsproxy](https://github.com/melihbirim/corsproxy), carrying on from its 1.2.6 release.

## Features

- **One static binary**, using only Go's standard library, for Linux, macOS and Windows, and a multi-arch container image of under 15 MB that runs as an unprivileged user.
- **A shared response cache**, off until you set `CACHE_TTL`: each upstream answer is fetched once for every client, revalidated cheaply when it expires, and served stale when the upstream refuses, as GitHub does once its rate limit is reached.
- **A GitHub token**, if you give it one, for GitHub's API only: 5,000 requests an hour instead of the 60 that every client of the proxy otherwise shares.
- **Locked down when you need it**: allowed origins, allowed and blocked hosts, a URL allowlist (with a report-only mode to build it), API keys, a per-IP rate limit and a daily request ceiling.
- **Read-only**: only `GET` is proxied, and a request body is never forwarded.
- **Safe by default**: private, loopback and cloud metadata addresses are refused on every connection, redirects included; credentials (`Authorization`, `Cookie`, `X-API-Key`) are never forwarded; cookies from upstream are dropped; proxied content cannot run as a page of the proxy's origin.
- **Honest about sizes**: responses over the limit are refused, never cut short.

## Quick start

```shell
docker run -d -p 8080:8080 \
  -e ALLOWED_ORIGINS=https://foundry.example.com \
  -e ALLOWED_HOSTS=foundryvtt.com,api.github.com,github.com,githubusercontent.com,gitlab.com \
  -e REQUIRE_API_KEY=true -e API_KEYS=change-me \
  -e CACHE_TTL=1h \
  ghcr.io/ulenarofmondarth/fvtt-package-corsproxy:latest

curl -H "X-API-Key: change-me" "http://localhost:8080/?url=https://api.github.com/repos/octocat/Hello-World"
```

Then give your module the proxy URL `https://<your proxy>/?url={url}` and the key. Put the proxy behind HTTPS before exposing it.

The [documentation](https://ulenarofmondarth.github.io/fvtt-package-corsproxy/) covers installing (container, release binary with systemd and nginx, or from source), every setting, the production checklist, and using it with Foundry VTT. To work on the proxy, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

MIT. See [LICENSE](LICENSE).

## AI Disclosure

The original proxy is [Melih Birim's work](https://github.com/melihbirim/corsproxy). This fork's tooling, configuration, documentation and changes were written with AI assistance (Claude Opus 5.5). All changes, whether written by a person or with AI assistance, are reviewed by a human before they are merged.
