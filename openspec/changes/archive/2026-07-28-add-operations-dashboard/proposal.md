## Why

Los datos de operaciones y salud de proxies serán útiles sólo si se pueden inspeccionar rápidamente. Hace falta un panel gráfico sin pipeline frontend, alojado en el mismo proceso que la API y preparado para una autenticación propia posterior.

## What Changes

- Servir un panel HTML en `/operations` y una API JSON bajo `/operations/api/*`, fuera del prefijo `/v1` y de la autenticación bearer de la API.
- Usar Tailwind CSS y Chart.js desde CDN, sin Node.js ni build previo.
- Mostrar operaciones activas, histórico de éxitos y errores, detalle expandible, estado y evolución de proxies, y gráficas de volumen, fallos y latencia para 24 horas, 7 días y 30 días.
- Ocultar la sección de proxies cuando no existan proxies configurados y excluir las rutas del panel de la instrumentación.
- Dejar el panel inicialmente sin autenticación propia; usuarios, contraseñas y sesiones SQLite quedan fuera de este cambio posterior.

## Capabilities

### New Capabilities

- `operations-dashboard`: Panel HTML y API de consulta para la telemetría operacional persistida.

### Modified Capabilities

- Ninguna.

## Impact

- Depende de `add-operations-storage`, `add-operation-event-recording` y `add-proxy-health-probes`.
- Afecta el agrupamiento de middleware/rutas del servidor, DI, OpenAPI, pruebas de contrato y README.
- Añade contenido HTML/CSS/JS embebido o servido por Go y dependencias CDN sólo en tiempo de navegador.
