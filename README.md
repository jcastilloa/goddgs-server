# goddgs-server

HTTP REST server for [goddgs](https://github.com/jcastilloa/goddgs), with one stable `goddgs` client per proxy and per-request rotation.

## Run

```sh
cp config.sample.yaml config.yaml
go run ./cmd/api
```

Configuration is loaded from `./config.yaml` or `~/.config/goddgs-server/config.yaml`. Environment variables follow Viper's naming convention; for example, `SERVICE_PORT=8081`.

## API

All search routes use `GET` and the `service.api_prefix` prefix (default: `/v1`).

- `/v1/text`
- `/v1/images`
- `/v1/news`
- `/v1/videos`
- `/v1/books`
- `/v1/extract`

Search endpoints accept `q` (or `query`), `region`, `safesearch`, `timelimit`, `max_results`, `page`, and `backend`. Images additionally accepts `size`, `color`, `type_image`, `layout`, and `license_image`; videos accepts `resolution`, `duration`, and `license_videos`. `extract` accepts `url` and `format`.

Search results are returned without narrowing `goddgs` types: numbers, nested maps, and null values are preserved. Documentation is served at `/docs/`, and the OpenAPI specification at `/openapi.json`.

If `auth.token` is not empty, every route requires:

```text
Authorization: Bearer <token>
```

## Proxies

`proxies` is required and must contain at least one uniquely named entry. Each entry creates one persistent `goddgs` client, so its outbound route and browser identity remain consistent for the lifetime of the process. The server selects healthy entries round-robin. A transport failure marks that entry unhealthy and can retry another entry; rate limits do not force rotation.

### Choose a configuration

| Situation | Entry type | What to configure |
| --- | --- | --- |
| No proxy | `direct` | Omit `url`. This is the normal local-development configuration. |
| One existing HTTP(S) or SOCKS proxy | `direct` | Set `url` to that proxy URL. |
| Tor Browser | `direct` | Set `url: tb`; it resolves to `socks5h://127.0.0.1:9150`. |
| One SSH egress host | `ssh` | Set host, user, private-key path, and verified host key. |
| Rotation | mixed | Configure two or more `direct` and/or `ssh` entries. |

### No proxy: direct host connection

Use this when the server can access the Internet itself. `url` must be omitted; do not set it to a placeholder such as `127.0.0.1:9050` unless a SOCKS server actually listens there.

```yaml
service:
  host: 0.0.0.0
  port: 8080
  api_prefix: /v1
  request_timeout: 30s
  max_proxy_retries: 1

auth:
  token: ""

proxies:
  - name: direct
    type: direct
```

The underlying library honors `DDGS_PROXY` when `url` is omitted. Leave `DDGS_PROXY` unset for a true direct connection:

```sh
unset DDGS_PROXY
```

### Existing direct proxy

`url` accepts `http://`, `https://`, `socks5://`, `socks5h://`, and `tb`. Use `socks5h` when DNS resolution must happen at the proxy rather than on this host.

```yaml
proxies:
  - name: company-connect
    type: direct
    url: http://proxy.example.net:3128

  - name: remote-dns-socks
    type: direct
    url: socks5h://proxy.example.net:1080
```

`tb` is a shortcut for the Tor Browser SOCKS listener at `127.0.0.1:9150`; it is not the usual Tor daemon port `9050`.

```yaml
proxies:
  - name: tor-browser
    type: direct
    url: tb
```

### SSH tunnel

For an `ssh` entry, the server creates and supervises its own loopback SOCKS5H listener. Do not configure the local SOCKS port: the operating system assigns it and the server keeps it stable while the SSH connection reconnects. The destination hostname is resolved on the SSH host.

```yaml
proxies:
  - name: madrid-egress
    type: ssh
    host: proxy.example.net
    port: 22 # Optional; defaults to 22.
    user: deploy
    private_key_path: /run/secrets/proxy_ed25519
    host_key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... proxy.example.net"
```

`host_key` is mandatory and is verified with `ssh.FixedHostKey`; `InsecureIgnoreHostKey` is never used. The private key path is read by the server process, so it must be readable by that process and should not be committed.

### Mixed rotating pool

The following configuration rotates requests between a direct connection, an existing SOCKS proxy, and an SSH egress host. Give each entry a unique `name`.

```yaml
proxies:
  - name: direct
    type: direct

  - name: socks-eu
    type: direct
    url: socks5h://socks.example.net:1080

  - name: madrid-egress
    type: ssh
    host: proxy.example.net
    port: 22
    user: deploy
    private_key_path: /run/secrets/proxy_ed25519
    host_key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... proxy.example.net"
```

The server tries the next healthy entry after a transport failure, up to `service.max_proxy_retries`. If every entry is unhealthy, the API returns `503` with `no healthy upstream connection available`. Connection failures return a descriptive `502` and are logged with the HTTP method, path, status, and underlying cause.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
```
