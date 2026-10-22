# Architecture

The proxy is one Go package, `main`, using only the standard library.

```{mermaid}
flowchart LR
  client[Browser] -->|"/?url=…"| handler[corsProxyHandler]
  handler --> checks{"API key, daily limit,<br>rate limit, URL, host"}
  checks -->|refused| error[4xx / 5xx from the proxy]
  checks -->|allowed| cache{Response cache}
  cache -->|fresh| response
  cache -->|missing or expired| transport[Shared transport<br>safe dialer]
  transport --> upstream[Upstream]
  upstream --> response[Response, with CORS headers]
  response --> client
```

## A request

`corsProxyHandler` (`main.go`) handles every request except `/health`, in this order:

1. sets the CORS headers, and answers a preflight (`OPTIONS`) at once;
2. refuses any method but `GET` with `405`, before anything that spends a budget;
3. checks the API key, before anything that spends a shared budget, so an unauthenticated caller cannot use it up;
4. counts the request against the daily limit, then the per-IP rate limit;
5. checks the `url` parameter and its host against `BLOCKED_HOSTS` and `ALLOWED_HOSTS`;
6. answers a `GET` from the response cache (`cache.go`), when it is on and has a fresh answer;
7. otherwise fetches it (`fetchUpstream`): builds the upstream request, a `GET` with no body, forwarding every header except `Host`, `Authorization`, `Cookie` and `X-API-Key`, adds the GitHub token for GitHub's API (`github.go`), sends it through the shared transport, following redirects only to allowed hosts, and reads the whole response, refusing it if it is larger than `MAX_REQUEST_SIZE`;
8. copies the status, headers (without `Set-Cookie`) and body back, with the sandboxing headers, and `X-Cache` and `Age` when the cache is on.

## The response cache

`memoryCache` (`cache.go`) is an LRU of whole answers, bounded by `CACHE_MAX_BYTES`, keyed on the target URL with the request's `Accept` and `Accept-Encoding`. Its `get` either serves a fresh answer, waits on the fetch already under way for the same key, or becomes that fetch itself, so a key is only ever fetched once at a time. `refresh` makes the fetch, conditional when an expired answer has an `ETag` or `Last-Modified`, and decides what to keep and what to serve: a `304` renews the expired answer, a refusal or failure serves it stale within `CACHE_MAX_STALE`, and anything else replaces it, or removes it if it cannot be kept.

The cache keeps answers marked `Cache-Control: private`, and honours only `no-store`. GitHub marks every authenticated answer `private`, but the proxy's token reads public repositories only and clients' credentials are never forwarded, so the answers are the same for every client.

The checks before it (API key, limits, host) run on every request, hits included, so the cache never answers a request the proxy would refuse. Its clock is a field, `now`, so the tests move time by hand.

## Safety

- **One shared transport** (`transport()`) for every upstream request, so connections are reused.
- **A safe dialer** (`safeDialer()`) checks the address actually connected to, after DNS resolution, on every connection. That refuses private addresses even through a redirect or DNS rebinding, which checking the hostname alone cannot.
- **Bounded state**: the rate limiter drops expired windows once it tracks more than 10,000 IPs.

## The GitHub token

`authenticateGitHub` (`github.go`) sets `Authorization: Bearer <GITHUB_TOKEN>` on a request whose host is `api.github.com` (`githubAPIHost`), and removes `Authorization` from any other. It runs on the first request and again on every redirect, in `CheckRedirect`. That matters because Go keeps `Authorization` on a redirect to a subdomain of the original host, and the token is for the API host alone.

## Configuration

`loadConfig()` reads every setting from the environment once, at start-up, into `config`. A new setting needs:

- a field in `Config` and a line in `loadConfig()`;
- a line in `.env.example` and in `make config`;
- a row in {doc}`../user/configuration`;
- an entry in `warnOnOpenDefaults()`, if leaving it off is unsafe.
