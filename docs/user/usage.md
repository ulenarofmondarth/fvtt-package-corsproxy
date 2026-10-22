# Endpoints

## `/?url=<target-url>`

Fetches the target URL and returns its response with CORS headers added. Only `GET` is proxied, and a request body is never forwarded: the proxy reads from upstreams, and never sends anything to one on a caller's behalf. `OPTIONS` (a preflight) is answered by the proxy itself, and any other method gets `405 Method Not Allowed`.

The target is the `url` query parameter. It must start with `http://` or `https://`, and be URL-encoded when it has a query of its own:

```shell
curl "http://localhost:8080/?url=https%3A%2F%2Fapi.github.com%2Frepos%2Foctocat%2FHello-World%2Freleases%3Fper_page%3D100"
```

From JavaScript:

```javascript
const target = 'https://api.github.com/repos/octocat/Hello-World/releases';
const response = await fetch(`http://localhost:8080/?url=${encodeURIComponent(target)}`, {
  headers: { 'X-API-Key': 'key-for-my-games' },
});
const releases = await response.json();
```

The upstream's status code, body and headers are returned as they are, apart from:

- the CORS headers, which the proxy sets;
- `Set-Cookie`, which is dropped, so an upstream cannot set cookies on the proxy's domain;
- `Content-Security-Policy: sandbox; default-src 'none'` and `X-Content-Type-Options: nosniff`, which are added, so that a proxied page can never run as a page of the proxy's origin.

The request's headers are forwarded, except `Host`, `Authorization`, `Cookie` and `X-API-Key`. Those carry credentials: forwarding them would let anyone use the proxy to replay someone else's token or cookie to another site, or leak the proxy's own API key to the target.

### Responses from the proxy itself

| Status | Body                                       | Why                                                   |
| ------ | ------------------------------------------ | ----------------------------------------------------- |
| `400`  | `Missing 'url' parameter…`                 | No `url`                                              |
| `400`  | `URL must start with http:// or https://`  | The target is not an HTTP URL                         |
| `405`  | `Only GET requests are proxied`            | A method other than `GET` (or the `OPTIONS` preflight) |
| `401`  | `Missing or invalid API key`               | `REQUIRE_API_KEY` is on, and the key is missing or wrong |
| `403`  | `This host is not allowed`                 | The target, or a redirect, is not allowed             |
| `403`  | `This URL is not allowed`                  | The target matches no `ALLOWED_URLS` pattern (a redirect to one gets `502`) |
| `429`  | `Rate limit exceeded…`                     | `RATE_LIMIT_PER_MINUTE` was reached                   |
| `502`  | `Failed to fetch URL…`, `…too large`       | The upstream could not be reached, or answered too much |
| `503`  | `Daily request limit reached…`             | `DAILY_REQUEST_LIMIT` was reached                     |

Any other status is the upstream's.

## `/health`

For uptime monitoring. Needs no API key.

```shell
curl http://localhost:8080/health
```

```json
{ "status": "ok", "version": "v1.3.0", "timestamp": "2026-10-22T12:00:00Z" }
```
