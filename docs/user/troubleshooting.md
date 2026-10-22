# Troubleshooting

## Many 403 responses

Look at the body of one:

- `{"error":"This host is not allowed"}` comes from the proxy: add the host to `ALLOWED_HOSTS` (see {doc}`foundry`).
- `API rate limit exceeded for <ip>` comes from GitHub, passed on by the proxy: every client of the proxy shares its 60 requests an hour (see {ref}`github-rate-limit`). Turn on the cache, and set a {ref}`GitHub token <github-token>`.

(token-refused-by-organization)=

## 403 from GitHub for one organization's repositories

With `GITHUB_TOKEN` set, GitHub can refuse the token for one organization's repositories while others work. The response still shows the token's limit (`X-Ratelimit-Limit: 5000`), and its `message` says why, for example:

> The 'Mondarth' organization forbids access via a fine-grained personal access tokens if the token's lifetime is greater than 366 days.

The organization sets a policy on fine-grained tokens, here a maximum lifetime, and applies it to its public repositories too. Regenerate the token to meet it (the message links to the token's settings), with an expiry date within a year, and give the proxy the new value. The refusal is not cached, so the next request after that reaches GitHub.

## 401 Bad credentials from GitHub

GitHub refused the proxy's `GITHUB_TOKEN`: it is wrong, has expired or has been revoked. Generate a new one (see {ref}`github-token`). Until then, unset `GITHUB_TOKEN` to fall back to unauthenticated requests.

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
