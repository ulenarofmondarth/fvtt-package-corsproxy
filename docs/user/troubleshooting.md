# Troubleshooting

## Many 403 responses

Look at the body of one:

- `{"error":"This host is not allowed"}` comes from the proxy: add the host to `ALLOWED_HOSTS` (see {doc}`foundry`).
- `API rate limit exceeded for <ip>` comes from GitHub, passed on by the proxy: every client of the proxy shares its 60 requests an hour (see {ref}`github-rate-limit`).

## The port is already in use

```shell
PORT=3000 ./bin/cors-proxy
```

## Requests time out

Upstream requests are given `REQUEST_TIMEOUT`, 30 seconds by default. Raise it for slow upstreams, and keep any reverse proxy's timeout a little above it.

## The Docker build fails

```shell
docker builder prune
docker build --no-cache -t cors-proxy .
```
