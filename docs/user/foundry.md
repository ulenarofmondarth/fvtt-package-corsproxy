# Using it with Foundry VTT

A Foundry module running in the browser can only read responses that carry CORS headers. Several of the places Foundry packages are published do not send them:

- Foundry's package listing and release notes (`foundryvtt.com`, `r2.foundryvtt.com`);
- GitHub's release downloads (`github.com`, and the `githubusercontent.com` hosts they redirect to);
- many of the hosts that manifest URLs point at.

GitHub's and GitLab's APIs do send CORS headers, but going through the proxy lets it share their answers between every client of the proxy.

## Allowed hosts

Set `ALLOWED_HOSTS` to the hosts the module looks packages up on. For package version lookups, such as EMMA's:

```shell
ALLOWED_HOSTS=foundryvtt.com,api.github.com,github.com,githubusercontent.com,gitlab.com
```

`foundryvtt.com` covers `r2.foundryvtt.com`, and `githubusercontent.com` covers `objects.`, `release-assets.` and `raw.githubusercontent.com`. Add the hosts of any other manifest URLs your packages use. A request for a host that is not allowed gets `403` with `{"error":"This host is not allowed"}`.

Set `ALLOWED_ORIGINS` to the origin your players open Foundry at, such as `https://foundry.example.com`, and turn on `REQUIRE_API_KEY` (see {doc}`configuration`).

## Setting it up in EMMA

In Foundry, open **Game Settings** → **Configure Settings** → EMMA's **Version lookup proxy** → **Set proxy** (GMs only). Give:

- the **proxy URL**, with `{url}` where the address to fetch goes: `https://proxy.example.com/?url={url}`;
- the **API key**, if the proxy requires one. EMMA keeps it in the GM's browser, and sends it in the `X-API-Key` header.

(github-rate-limit)=

## GitHub's rate limit

GitHub allows 60 unauthenticated API requests an hour per IP address. Every client of the proxy shares the proxy's IP address, so they share those 60 requests too. A request over the limit gets GitHub's own `403`, `API rate limit exceeded for <ip>`, which the proxy passes on. That is different from the proxy's own `403` for a host that is not allowed: compare the response bodies.

Turn on the {ref}`response cache <response-cache>` to stay well inside the limit:

```shell
CACHE_TTL=1h
```

Each release list is then fetched from GitHub at most once an hour, whichever clients ask for it, and revalidating an unchanged one costs little. When GitHub refuses all the same, clients are given the last answer it gave, for up to `CACHE_MAX_STALE`, rather than the `403`.

:::{todo}
Authenticate to GitHub with a token, to raise the limit to 5,000 requests an hour.
:::
