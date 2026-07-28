## 1. Rutas, autenticación y casos de uso

- [x] 1.1 Reorganizar el middleware para aplicar bearer sólo a `/v1`, `/openapi.json` y `/docs`, dejando el grupo `/operations` fuera de ese bearer.
- [x] 1.2 Añadir rutas, handlers y registros DI para el panel HTML y la API JSON de operaciones.
- [x] 1.3 Implementar casos de uso de resumen, serie temporal, listado paginado, detalle y proxies sobre el puerto de consulta SQLite.
- [x] 1.4 Validar rangos, fechas, intervalos, filtros y límites, con respuestas 400, 404 y 500 coherentes.
- [x] 1.5 Excluir `/operations` y `/operations/api/*` de la instrumentación de operaciones.

## 2. Interfaz sin build

- [x] 2.1 Crear y embeber el documento HTML de `/operations` con Tailwind CDN y Chart.js CDN, sin archivos compilados.
- [x] 2.2 Implementar refresco periódico mediante `fetch`, manejo de errores y selección de rango de 24 horas, 7 días o 30 días.
- [x] 2.3 Renderizar indicadores, operaciones activas, tabla de recientes, detalle expandible y estados vacíos.
- [x] 2.4 Renderizar gráficas de volumen éxito/error, p50/p95 y salud/latencia de proxies.
- [x] 2.5 Ocultar la sección de proxies cuando no haya proxies o resultados de sonda.

## 3. Contrato, pruebas y documentación

- [x] 3.1 Escribir primero pruebas de handlers para validación, paginación, rangos, operación inexistente y datos vacíos.
- [x] 3.2 Añadir pruebas de servidor que comprueben acceso a `/operations` sin bearer y preservación de auth en `/v1`, `/openapi.json` y `/docs`.
- [x] 3.3 Actualizar `platform/server/openapi.go` y sus pruebas de contrato con todas las rutas del panel, parámetros, ejemplos, errores y `security: []`.
- [x] 3.4 Actualizar README con rutas, datos mostrados, configuración SQLite/sondas, CDN y autenticación propia pendiente.
- [x] 3.5 Ejecutar `gofmt`, `go test ./...`, `go test -race ./...` y `go vet ./...`.
