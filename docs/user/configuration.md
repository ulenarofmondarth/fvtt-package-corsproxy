# Configuration

The proxy is configured entirely through environment variables. `.env.example` in the repository lists them all, and `make config` shows the values the current environment would give.

## Environment variables

| Variable                 | Default    | Description                                                                                                       |
| ------------------------ | ---------- | ----------------------------------------------------------------------------------------------------------------- |
| `PORT`                   | `8080`     | The port to listen on                                                                                             |
| `MAX_REQUEST_SIZE`       | `10485760` | The largest request body, and upstream response, in bytes (10 MB)                                                 |
| `REQUEST_TIMEOUT`        | `30s`      | How long an upstream request may take (a Go duration: `30s`, `1m`)                                                |
| `MAX_REDIRECTS`          | `10`       | The most redirects followed                                                                                       |
| `ALLOWED_ORIGINS`        | `*`        | The origins allowed to call the proxy: `*` for any, or a comma-separated list                                     |
| `ALLOWED_HOSTS`          |            | The hosts the proxy may fetch from, comma-separated; empty allows any public host                                 |
| `BLOCKED_HOSTS`          |            | Hosts the proxy never fetches from, comma-separated                                                               |
| `ALLOW_PRIVATE_NETWORKS` | `false`    | Allow fetching from loopback, private and link-local addresses (trusted internal deployments only)                |
| `RATE_LIMIT_PER_MINUTE`  | `0`        | Requests per minute per client IP; `0` turns it off                                                               |
| `TRUST_PROXY_HEADERS`    | `false`    | Take the client IP from `X-Forwarded-For`, for rate limiting; only behind a reverse proxy that sets it (see below) |
| `REQUIRE_API_KEY`        | `false`    | Require one of `API_KEYS` on every proxied request                                                                |
| `API_KEYS`               |            | The accepted API keys, comma-separated                                                                            |
| `DAILY_REQUEST_LIMIT`    | `0`        | The most proxied requests per rolling 24 hours, across all clients; `0` turns it off                              |
| `VERBOSE_LOGGING`        | `false`    | Log every request                                                                                                 |

## Production checklist

The defaults are wide open, so that `go run .` works with no setup. Before exposing the proxy on the internet:

- **Set `ALLOWED_ORIGINS`** to your Foundry server's origin, such as `https://foundry.example.com`. The default `*` lets any website's JavaScript use your proxy.
- **Set `ALLOWED_HOSTS`** to the hosts you intend to fetch from (see {doc}`foundry`). An empty list lets the proxy reach any public host, which makes it attractive as a free open relay.
- **Set `RATE_LIMIT_PER_MINUTE`** to bound abuse from one IP, and set `TRUST_PROXY_HEADERS=true` with it if, and only if, the proxy is behind a reverse proxy or platform that sets `X-Forwarded-For`. Otherwise anyone can bypass the limit by sending the header themselves.
- **Set `DAILY_REQUEST_LIMIT`** as a cost backstop, if you pay for traffic or compute.
- **Turn on `REQUIRE_API_KEY`** when the proxy serves your own games rather than the public.
- **Leave `ALLOW_PRIVATE_NETWORKS` off**, unless the proxy must reach internal addresses: otherwise it is an SSRF hole.
- **Run it behind HTTPS.**
- **Watch the startup log.** A `⚠️  Running with open defaults` line lists which of these are still off.

## Hosts

```shell
# Only these hosts, and their subdomains
ALLOWED_HOSTS=api.github.com,githubusercontent.com

# Never these hosts, and their subdomains (checked first)
BLOCKED_HOSTS=internal.example.com
```

A rule matches the host and its subdomains: `githubusercontent.com` matches `objects.githubusercontent.com`, but not `githubusercontent.com.evil.net`. Redirects are checked against the same rules.

Private, loopback, link-local (including the cloud metadata address 169.254.169.254) and CGNAT addresses are always refused, checked on the resolved IP of every connection, redirects included. Set `ALLOW_PRIVATE_NETWORKS=true` only if the proxy must reach internal services.

## Rate limiting

```shell
# 100 requests per minute per client IP
RATE_LIMIT_PER_MINUTE=100

# Only behind a reverse proxy or platform (nginx, a CDN, Railway, Render, Fly) that sets or
# overwrites X-Forwarded-For itself.
TRUST_PROXY_HEADERS=true
```

`TRUST_PROXY_HEADERS` is off by default. Without a trusted proxy in front, a client can send any `X-Forwarded-For` it likes and get a new rate-limit window with every request.

## API keys

```shell
# Require a key on every proxied request (the health check needs none)
REQUIRE_API_KEY=true
API_KEYS=key-for-my-games,key-for-testing
```

Callers send the key in an `X-API-Key` header, or an `apikey` query parameter:

```shell
curl -H "X-API-Key: key-for-my-games" "https://proxy.example.com/?url=https://api.github.com"
curl "https://proxy.example.com/?url=https://api.github.com&apikey=key-for-my-games"
```

Prefer the header: a query parameter ends up in access logs. A request with no key, or the wrong one, gets `401`.

## Daily request limit

```shell
# Stop at 100,000 proxied requests per rolling 24 hours, across every client
DAILY_REQUEST_LIMIT=100000
```

The count is kept in memory, so it resets when the proxy restarts, and each replica keeps its own. It is a cost cap for a single instance, not a shared rate limiter. Once it is reached, requests get `503` until the window ends.

## Request limits

```shell
MAX_REQUEST_SIZE=5242880  # 5 MB
REQUEST_TIMEOUT=15s
```

An upstream response larger than `MAX_REQUEST_SIZE` is refused with `502`, never cut short.
