## Context

El servidor Gin aplica actualmente autenticación global, incluidas documentación y OpenAPI. Los cambios anteriores aportarán datos SQLite de operaciones y sondas. El panel debe convivir en el mismo host/puerto, no usar el bearer token de `/v1`, no requerir build frontend y se autenticará con un mecanismo propio en un cambio posterior.

## Goals / Non-Goals

**Goals:**

- Servir `/operations` y consultas JSON desde el mismo proceso y dirección HTTP.
- Separar la autenticación bearer del grupo API sin cambiar la protección de `/v1`, `/openapi.json` y `/docs`.
- Presentar datos actuales e históricos con Tailwind y Chart.js desde CDN.
- Documentar todos los endpoints JSON públicos del panel en OpenAPI y probar su contrato.

**Non-Goals:**

- Implementar usuarios, contraseñas, sesiones o autorización del panel.
- Introducir Node, npm, compilación de assets, WebSockets o SSE.
- Exponer datos que no se hayan persistido o modificar los contratos API de negocio.

## Decisions

### Grupos de rutas y middleware explícitos

El engine conservará recuperación, logging y timeout globales; la autenticación bearer se aplicará exclusivamente al grupo API y a documentación. `/operations` y `/operations/api` se registrarán directamente sin ese middleware, manteniendo de forma consciente la excepción temporal acordada. El middleware de instrumentación excluirá todo el prefijo `/operations` para evitar autoobservación.

### HTML estático servido desde Go y dependencias CDN

Un handler devuelve el documento HTML de una sola página. Carga Tailwind mediante `https://cdn.tailwindcss.com` y Chart.js desde su CDN oficial con `defer`; el JavaScript usa `fetch` periódico de JSON cada cinco segundos, maneja errores de red y no necesita ningún asset generado. Se preferirá `go:embed` para que el contenido viaje con el binario sin dependencias de paths de despliegue.

### API de lectura pequeña y paginada

Se publicarán endpoints JSON para resumen, series temporales, lista de operaciones, detalle de una operación y proxies. Fechas ISO-8601, rangos `24h`, `7d` o `30d`, intervalos seguros y límites paginados se validarán en el handler; las consultas delegan en el caso de uso de operaciones. El detalle no incluirá secretos por las garantías de saneamiento del registrador.

### Visualización resiliente y datos vacíos

El panel mostrará tarjetas de resumen, tabla de operaciones activas/recientes y gráficas de volumen éxito/error, p50/p95 y proxy. Si no hay registros, cada componente muestra un estado vacío. Si no hay proxies configurados o no hay datos de sondas, la sección de proxies se oculta sin afectar el resto del panel.

### OpenAPI de operaciones sin bearer

`/operations` se documentará como respuesta HTML; las rutas `/operations/api/*` incluirán parámetros, valores por defecto, ejemplos y respuestas 400/404/500. Como la especificación hoy tiene seguridad global, las operaciones del panel declararán `security: []` para expresar que no usan el bearer de API durante esta fase. README advertirá que la protección propia está pendiente.

## Risks / Trade-offs

- [Panel accesible en todas las interfaces antes del login propio] → comportamiento aceptado explícitamente; README lo documenta y el diseño permite añadir middleware del panel sin cambiar sus handlers.
- [CDN no disponible] → la API JSON sigue accesible; el HTML mostrará un fallo claro en lugar de requerir un build local.
- [Consultas de 30 días demasiado pesadas] → agregación SQL por intervalo, límites máximos y paginación.
- [La separación de auth abre rutas existentes] → pruebas de regresión para `/v1`, `/openapi.json` y `/docs` con token configurado.

## Migration Plan

1. Reorganizar middleware y registrar handlers/DI de operaciones sin cambiar contratos de `/v1`.
2. Implementar casos de uso y API JSON paginada sobre el puerto de consulta SQLite.
3. Añadir HTML embebido y refresco con CDN; verificar el modo sin datos ni proxies.
4. Actualizar OpenAPI, pruebas de contrato y README.

Se puede revertir quitando el grupo `/operations`; los datos SQLite y la instrumentación continúan sin efecto visible.

## Open Questions

- Ninguna para la primera versión. Un cambio posterior añadirá autenticación propia con usuarios y contraseñas hash en SQLite antes de considerar el panel expuesto de manera general.
