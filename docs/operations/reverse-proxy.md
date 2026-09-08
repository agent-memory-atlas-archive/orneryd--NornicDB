# Reverse Proxy and TLS Termination

NornicDB supports an authenticated HTTP deployment behind a trusted reverse
proxy. The proxy accepts HTTPS from clients and forwards HTTP to NornicDB on a
private connection.

The trust boundary is explicit. NornicDB accepts `X-Forwarded-*`, `X-Real-IP`,
and forwarded-prefix headers only when the request's immediate peer matches
`NORNICDB_HTTP_TRUSTED_PROXIES`. Forwarding headers from every other peer are
removed before routing, authentication, cookie, audit, and rate-limit logic.

## Required NornicDB settings

For a proxy on another container or host:

```bash
NORNICDB_ADDRESS=0.0.0.0
NORNICDB_HTTP_TRUSTED_PROXIES=172.20.0.10/32
NORNICDB_CORS_ENABLED=false
NORNICDB_BOLT_ENABLED=false
NORNICDB_QDRANT_GRPC_ENABLED=false
```

Replace `172.20.0.10/32` with the source IP or narrow CIDR that NornicDB sees
for the proxy connection. Hostnames are not accepted. Multiple entries are
comma-separated:

```bash
NORNICDB_HTTP_TRUSTED_PROXIES=172.20.0.10/32,172.21.0.0/24
```

For a proxy running on the same machine, keep NornicDB on loopback and trust
only loopback:

```bash
NORNICDB_ADDRESS=127.0.0.1
NORNICDB_HTTP_TRUSTED_PROXIES=127.0.0.1/32,::1/128
NORNICDB_CORS_ENABLED=false
```

When the browser application has a different origin from NornicDB, enable CORS
and list exact HTTPS origins instead of disabling it:

```bash
NORNICDB_CORS_ENABLED=true
NORNICDB_CORS_ORIGINS=https://console.example.com
```

Authenticated startup rejects wildcard CORS on a non-loopback listener. It also
rejects cleartext HTTP on a non-loopback listener unless trusted proxies are
configured. Declaring trusted proxies does not create a firewall: restrict the
backend port so only those proxies can reach it.

`NORNICDB_HTTP_TRUSTED_PROXIES` applies only to HTTP. If Bolt is enabled on a
non-loopback address, configure native Bolt TLS and require it. If Qdrant gRPC
is enabled, bind it to loopback or provide a separately secured transport.

## Nginx configuration

This example terminates TLS at Nginx and deliberately overwrites client-supplied
forwarding headers:

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

upstream nornicdb_http {
    server 172.20.0.20:7474;
}

server {
    listen 443 ssl http2;
    server_name db.example.com;

    ssl_certificate     /etc/nginx/tls/fullchain.pem;
    ssl_certificate_key /etc/nginx/tls/privkey.pem;

    location / {
        proxy_pass http://nornicdb_http;
        proxy_http_version 1.1;

        proxy_set_header Host              $http_host;
        proxy_set_header X-Forwarded-Host  $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Port  $server_port;
        proxy_set_header X-Forwarded-For   $remote_addr;
        proxy_set_header X-Real-IP         $remote_addr;

        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
```

Use `$remote_addr`, not `$proxy_add_x_forwarded_for`, at this trust boundary so
a client cannot prepend a false address that becomes the apparent client IP.

Verify the public endpoint:

```bash
curl --fail --show-error https://db.example.com/health
curl --fail --show-error \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"REDACTED"}' \
  -D - https://db.example.com/auth/token -o /dev/null
```

The authentication response must include a `nornicdb_token` cookie with the
`Secure` and `HttpOnly` attributes.

## URL prefix

To expose NornicDB at `https://example.com/nornicdb`, set:

```bash
NORNICDB_BASE_PATH=/nornicdb
```

Preserve the prefix when proxying:

```nginx
location /nornicdb/ {
    proxy_pass http://nornicdb_http;
    proxy_set_header Host              $http_host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For   $remote_addr;
}
```

## Native HTTPS

If the backend connection must also be encrypted, enable NornicDB's HTTPS
listener instead of using trusted cleartext proxy mode:

```bash
NORNICDB_ADDRESS=0.0.0.0
NORNICDB_HTTPS_ENABLED=true
NORNICDB_HTTPS_PORT=7473
NORNICDB_HTTP_TLS_CERT=/tls/server.crt
NORNICDB_HTTP_TLS_KEY=/tls/server.key
NORNICDB_CORS_ENABLED=false
```

Equivalent YAML:

```yaml
server:
  http_address: "0.0.0.0"
  https:
    enabled: true
    port: 7473
    cert_file: /tls/server.crt
    key_file: /tls/server.key
```

When HTTPS is enabled, both certificate and key are required. The HTTPS port is
used unless `--http-port` is explicitly supplied.

## Troubleshooting startup

`security configuration: wildcard CORS origin is not allowed`

: Disable CORS for same-origin deployments or set exact HTTPS origins.

`security configuration: public plaintext HTTP listener is not allowed`

: Set `NORNICDB_HTTP_TRUSTED_PROXIES` for TLS termination at a trusted proxy, or
enable native HTTPS.

`security configuration: public Bolt listener must require TLS`

: Disable Bolt when only HTTP is proxied, bind it to loopback, or configure Bolt
TLS with `NORNICDB_BOLT_TLS_REQUIRE=true`.
