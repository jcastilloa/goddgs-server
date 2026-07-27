# goddgs-server

Servidor HTTP REST para [goddgs](https://github.com/jcastilloa/goddgs), con un cliente `goddgs` estable por proxy y rotación por petición.

## Ejecutar

```sh
cp config.sample.yaml config.yaml
go run ./cmd/api
```

La configuración se busca en `./config.yaml` o en `~/.config/goddgs-server/config.yaml`. Las variables de entorno siguen el patrón de Viper, por ejemplo `SERVICE_PORT=8081`.

## API

Todas las rutas de búsqueda usan `GET` y el prefijo `service.api_prefix` (por defecto `/v1`).

- `/v1/text`
- `/v1/images`
- `/v1/news`
- `/v1/videos`
- `/v1/books`
- `/v1/extract`

Los endpoints de búsqueda aceptan `q` (o `query`), `region`, `safesearch`, `timelimit`, `max_results`, `page` y `backend`. Imágenes acepta además `size`, `color`, `type_image`, `layout` y `license_image`; vídeos acepta `resolution`, `duration` y `license_videos`. `extract` recibe `url` y `format`.

Los resultados de búsqueda se devuelven sin estrechar los tipos de `goddgs`: se conservan números, mapas anidados y valores nulos. La documentación se sirve en `/docs/` y la especificación OpenAPI en `/openapi.json`.

Si `auth.token` no está vacío, todas las rutas requieren:

```text
Authorization: Bearer <token>
```

## Proxies

Cada proxy crea un cliente `goddgs` persistente; por tanto, proxy e identidad de navegador se mantienen coherentes durante la vida del proceso. El selector usa round-robin entre entradas sanas. Una caída de transporte marca la entrada no sana y puede reintentar otra; los rate limits no fuerzan rotación.

Los proxies `direct` admiten `http://`, `https://`, `socks5://`, `socks5h://` y `tb`. Para túneles `ssh`, el proceso abre un listener SOCKS5H en loopback con puerto asignado por el sistema. Ese puerto no cambia al reconectar SSH, de modo que el cliente `goddgs` no se reconstruye.

Ejemplo SSH:

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

La `host_key` es obligatoria y se valida con `ssh.FixedHostKey`; no se usa `InsecureIgnoreHostKey`.

## Verificación

```sh
go test ./...
go test -race ./...
go vet ./...
```
