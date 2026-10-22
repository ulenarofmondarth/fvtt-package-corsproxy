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
| `CACHE_TTL`              | `0`        | How long an upstream answer is served from the {ref}`response cache <response-cache>`; `0` turns it off           |
| `CACHE_NEGATIVE_TTL`     | `5m`       | The same, for a 404 or 410 answer                                                                                 |
| `CACHE_MAX_STALE`        | `24h`      | How long past `CACHE_TTL` an answer is still served when the upstream refuses or cannot be reached                |
| `CACHE_MAX_BYTES`        | `67108864` | The most memory the cache uses, in bytes (64 MB)                                                                  |
| `CACHE_MAX_ENTRY_BYTES`  | `2097152`  | The largest answer the cache keeps, in bytes (2 MB)                                                               |
| `VERBOSE_LOGGING`        | `false`    | Log every request                                                                                                 |

## Production checklist

The defaults are wide open, so that `go run .` works with no setup. Before exposing the proxy on the internet:

- **Set `ALLOWED_ORIGINS`** to your Foundry server's origin, such as `https://foundry.example.com`. The default `*` lets any website's JavaScript use your proxy.
- **Set `ALLOWED_HOSTS`** to the hosts you intend to fetch from (see {doc}`foundry`). An empty list lets the proxy reach any public host, which makes it attractive as a free open relay.
- **Set `RATE_LIMIT_PER_MINUTE`** to bound abuse from one IP, and set `TRUST_PROXY_HEADERS=true` with it if, and only if, the proxy is behind a reverse proxy or platform that sets `X-Forwarded-For`. Otherwise anyone can bypass the limit by sending the header themselves.
- **Set `DAILY_REQUEST_LIMIT`** as a cost backstop, if you pay for traffic or compute.
- **Turn on `REQUIRE_API_KEY`** when the proxy serves your own games rather than the public.
- **Set `CACHE_TTL`** if many clients look up the same things, such as package versions, so the upstreams see one request where they would have seen many.
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

(response-cache)=

## Response cache

```shell
# Serve each upstream answer for an hour before fetching it again
CACHE_TTL=1h
```

The cache keeps upstream answers in memory, shared by every client, so a URL many clients ask for is fetched once per `CACHE_TTL` instead of once per request. It is off unless `CACHE_TTL` is set.

What it keeps:

- answers to plain `GET` requests. A `POST`, or a request with a `Range` header or a condition of its own (`If-None-Match`, `If-Modified-Since` and the like), goes straight to the upstream;
- `200` answers for `CACHE_TTL`, and `404` and `410` answers for `CACHE_NEGATIVE_TTL`. Other answers are passed on and not kept;
- not answers that set cookies, are marked `Cache-Control: no-store`, or say `Vary: *`, nor any larger than `CACHE_MAX_ENTRY_BYTES`.

An answer is kept for its URL together with the request's `Accept` and `Accept-Encoding` headers. The proxy never forwards credentials, so nothing else can make two clients' answers differ.

Once an answer expires, the next request for it revalidates it: the proxy asks the upstream whether it has changed (`If-None-Match` with its `ETag`, or `If-Modified-Since`), and a `304` renews it without downloading it again. If the upstream refuses instead (`403`, as GitHub's rate limit does, `429` or a `5xx`) or cannot be reached, the expired answer is served, for up to `CACHE_MAX_STALE` past its expiry. A `404` replaces it.

While one request for an answer is with the upstream, others for the same answer wait for it rather than sending their own. When the cache is full, the least recently used answers make room.

Every answer says where it came from, in an `X-Cache` header that the client's script can read:

| `X-Cache`     | Meaning                                                               |
| ------------- | --------------------------------------------------------------------- |
| `MISS`        | Fetched from the upstream                                             |
| `HIT`         | Served from the cache                                                 |
| `REVALIDATED` | Expired, and the upstream confirmed it unchanged                      |
| `STALE`       | Expired, and served because the upstream refused or could not be reached |
| `BYPASS`      | Not a request the cache answers                                       |

Answers from the cache also carry `Age`, the seconds since the upstream last gave or confirmed them.

Clients cannot make the proxy skip the cache: it protects the upstreams' budgets, which every client shares. The cache lives in memory, so it is empty after a restart, and each replica keeps its own.
