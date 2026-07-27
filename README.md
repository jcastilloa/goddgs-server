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

Each proxy creates a persistent `goddgs` client, so the proxy and browser identity remain consistent throughout the process lifetime. The selector uses round-robin across healthy entries. A transport failure marks an entry unhealthy and may retry another entry; rate limits do not force rotation.

`direct` entries without a `url` use the host's direct connection unless `DDGS_PROXY` is set. Entries with a `url` support `http://`, `https://`, `socks5://`, `socks5h://`, and `tb`. For `ssh` tunnels, the process opens a loopback SOCKS5H listener on a system-assigned port. That port remains unchanged across SSH reconnects, so the `goddgs` client is not rebuilt.

SSH example:

```yaml
proxies:
  - name: madrid-egress
    type: ssh
    host: proxy.example.net
    port: 22
    user: deploy
    private_key_path: /run/secrets/proxy_ed25519
    host_key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... proxy.example.net"
```

`host_key` is mandatory and validated with `ssh.FixedHostKey`; `InsecureIgnoreHostKey` is never used.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
```
