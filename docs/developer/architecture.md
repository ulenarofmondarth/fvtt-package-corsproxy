# Architecture

The proxy is one Go package, `main`, using only the standard library.

```{mermaid}
flowchart LR
  client[Browser] -->|"/?url=…"| handler[corsProxyHandler]
  handler --> checks{"API key, daily limit,<br>rate limit, URL, host"}
  checks -->|refused| error[4xx / 5xx from the proxy]
  checks -->|allowed| transport[Shared transport<br>safe dialer]
  transport --> upstream[Upstream]
  upstream --> response[Response, with CORS headers]
  response --> client
```

## A request

`corsProxyHandler` (`main.go`) handles every request except `/health`, in this order:

1. sets the CORS headers, and answers a preflight (`OPTIONS`) at once;
2. checks the API key, before anything that spends a shared budget, so an unauthenticated caller cannot use it up;
3. counts the request against the daily limit, then the per-IP rate limit;
4. checks the `url` parameter and its host against `BLOCKED_HOSTS` and `ALLOWED_HOSTS`;
5. builds the upstream request, forwarding every header except `Host`, `Authorization`, `Cookie` and `X-API-Key`;
6. sends it through the shared transport, following redirects only to allowed hosts;
7. reads the response, refusing it if it is larger than `MAX_REQUEST_SIZE`, before writing anything;
8. copies the status, headers (without `Set-Cookie`) and body back, with the sandboxing headers.

## Safety

- **One shared transport** (`transport()`) for every upstream request, so connections are reused.
- **A safe dialer** (`safeDialer()`) checks the address actually connected to, after DNS resolution, on every connection. That refuses private addresses even through a redirect or DNS rebinding, which checking the hostname alone cannot.
- **Bounded state**: the rate limiter drops expired windows once it tracks more than 10,000 IPs.

## Configuration

`loadConfig()` reads every setting from the environment once, at start-up, into `config`. A new setting needs:

- a field in `Config` and a line in `loadConfig()`;
- a line in `.env.example` and in `make config`;
- a row in {doc}`../user/configuration`;
- an entry in `warnOnOpenDefaults()`, if leaving it off is unsafe.
