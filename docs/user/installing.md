# Installing

The proxy is a single static binary with no runtime dependencies. Every release publishes it for Linux, macOS and Windows, and as a multi-arch container image (linux/amd64, linux/arm64).

The proxy speaks plain HTTP. Anything reachable from the internet should sit behind HTTPS: a platform's load balancer, or a reverse proxy such as nginx or Caddy. Read {doc}`configuration` before exposing it; its defaults are wide open.

## Container image

```shell
# Latest release
docker run -d --name corsproxy -p 8080:8080 \
  -e ALLOWED_ORIGINS=https://foundry.example.com \
  -e ALLOWED_HOSTS=api.github.com \
  ghcr.io/ulenarofmondarth/fvtt-package-corsproxy:latest

# Pin a version (tags: 1.3.0, 1.3, 1, latest)
docker run -d -p 8080:8080 ghcr.io/ulenarofmondarth/fvtt-package-corsproxy:1.3.0

# Check what you are running
curl -s localhost:8080/health
# {"status":"ok","version":"v1.3.0","timestamp":"..."}
```

The image runs as an unprivileged user (uid 10001) and is under 15 MB.

The repository's `docker-compose.yml` builds and runs it from source, with the settings in the file:

```shell
docker compose up --build
```

## Release binary, systemd and nginx

Any Linux machine works. Download the release binary, run it as a service bound to localhost, and put nginx in front for TLS.

```shell
# 1. Binary (pick linux-amd64 or linux-arm64)
curl -fsSL -o /usr/local/bin/cors-proxy \
  https://github.com/ulenarofmondarth/fvtt-package-corsproxy/releases/latest/download/cors-proxy-linux-amd64
chmod +x /usr/local/bin/cors-proxy
useradd --system --no-create-home --shell /usr/sbin/nologin corsproxy

# 2. Configuration
cat > /etc/corsproxy.env <<'EOF'
PORT=8080
ALLOWED_ORIGINS=https://foundry.example.com
ALLOWED_HOSTS=api.github.com
RATE_LIMIT_PER_MINUTE=100
TRUST_PROXY_HEADERS=true
EOF
chmod 600 /etc/corsproxy.env
```

Each release also has `checksums.txt`, the SHA-256 of every binary, to check the download against (`sha256sum --check --ignore-missing checksums.txt`).

```ini
# 3. /etc/systemd/system/corsproxy.service
[Unit]
Description=CORS proxy
After=network-online.target
Wants=network-online.target

[Service]
User=corsproxy
EnvironmentFile=/etc/corsproxy.env
ExecStart=/usr/local/bin/cors-proxy
Restart=always
RestartSec=2
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```shell
systemctl daemon-reload && systemctl enable --now corsproxy
curl -s localhost:8080/health
```

```nginx
# 4. /etc/nginx/sites-available/corsproxy  (TLS via certbot or your own certificates)
server {
    listen 443 ssl http2;
    server_name proxy.example.com;

    ssl_certificate     /etc/letsencrypt/live/proxy.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/proxy.example.com/privkey.pem;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_read_timeout 35s;   # a bit above REQUEST_TIMEOUT
        client_max_body_size 10m; # match MAX_REQUEST_SIZE
    }
}
```

`TRUST_PROXY_HEADERS=true` is right here because nginx overwrites `X-Forwarded-For`. Leave it `false` if the binary can be reached directly.

## From source

You need Go 1.27 or later.

```shell
git clone https://github.com/ulenarofmondarth/fvtt-package-corsproxy.git
cd fvtt-package-corsproxy
make build          # bin/cors-proxy
./bin/cors-proxy
```

The server starts on `http://localhost:8080`.
