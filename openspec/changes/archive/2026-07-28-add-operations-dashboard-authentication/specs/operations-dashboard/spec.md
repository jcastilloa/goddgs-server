## MODIFIED Requirements

### Requirement: Rutas separadas del panel de operaciones
El sistema SHALL servir el panel HTML en `GET /operations` y su API JSON bajo `/operations/api/*` en el mismo host y puerto que la API. Estas rutas MUST requerir una sesión válida de dashboard y MUST NOT aceptar el bearer token configurado para `/v1` como alternativa. Las rutas API, OpenAPI y Swagger existentes MUST conservar su comportamiento de autenticación bearer.

Las solicitudes anónimas o con sesión inválida a `GET /operations` MUST recibir una redirección 303 al flujo de configuración inicial o login, según exista la cuenta. Las rutas JSON protegidas MUST responder 401 con `{"error":"dashboard authentication required"}` en la misma condición.

#### Scenario: Acceso anónimo al panel con bearer de API configurado
- **WHEN** `auth.token` está configurado y un cliente solicita `GET /operations` sin una sesión válida de dashboard
- **THEN** el servidor redirige 303 a setup o login sin exigir ni aceptar el bearer token

#### Scenario: API de operaciones sin sesión
- **WHEN** un cliente solicita una ruta `/operations/api/operations` sin una sesión válida de dashboard
- **THEN** el servidor responde 401 con el error de autenticación de dashboard documentado

#### Scenario: API existente con bearer configurado
- **WHEN** `auth.token` está configurado y un cliente solicita una ruta `/v1` sin credenciales
- **THEN** el servidor continúa respondiendo 401

### Requirement: API de consulta de operaciones
El sistema SHALL proporcionar endpoints JSON autenticados para resumen, serie temporal, operaciones paginadas, detalle por identificador y proxies. Los endpoints MUST requerir la sesión de dashboard, validar fechas, rangos, intervalos y límites, y MUST devolver datos saneados desde el almacenamiento de operaciones.

#### Scenario: Consulta de operaciones recientes autenticada
- **WHEN** un cliente con sesión de dashboard válida solicita `GET /operations/api/operations` con un límite válido
- **THEN** recibe una lista paginada de operaciones con estado, tipo, inicio, duración y resultado

#### Scenario: Parámetro temporal inválido
- **WHEN** un cliente autenticado solicita una serie temporal con un rango o intervalo no permitido
- **THEN** el servidor responde 400 con un error documentado

#### Scenario: Operación inexistente
- **WHEN** un cliente autenticado solicita una operación que no existe
- **THEN** el servidor responde 404 con un error documentado

### Requirement: Resumen y evolución operacional
El panel SHALL mostrar operaciones activas, totales de éxitos y errores, percentiles p50/p95 de duración y gráficas de volumen, fallos y latencia para los periodos de 24 horas, 7 días y 30 días. El panel MUST refrescar sus datos sin recargar la página mientras la sesión permanezca válida.

El panel autenticado MUST incluir en su zona superior derecha un indicador visible del nombre del usuario autenticado y controles accesibles para abrir el cambio de contraseña y cerrar sesión. Si una consulta de telemetría recibe 401, el cliente MUST dejar de mostrar datos protegidos y navegar al flujo de login.

#### Scenario: Datos de 24 horas disponibles
- **WHEN** una sesión válida abre el panel y existen operaciones finalizadas en las últimas 24 horas
- **THEN** el panel muestra los indicadores y las gráficas agregadas para ese periodo junto a la identidad de sesión

#### Scenario: Sin operaciones registradas
- **WHEN** una sesión válida abre el panel y no existen operaciones en el rango seleccionado
- **THEN** el panel muestra estados vacíos claros sin errores de JavaScript ni gráficas engañosas

#### Scenario: Caducidad durante el refresco
- **WHEN** una sesión caduca y el siguiente refresco de telemetría recibe 401
- **THEN** el panel redirige al login sin continuar mostrando datos operativos protegidos

### Requirement: Entrega sin build frontend
El sistema SHALL entregar el panel, la configuración inicial, el login y el cambio de contraseña como HTML servido por Go, con Tailwind CSS desde CDN y Chart.js desde CDN cuando corresponda. El proyecto MUST NOT requerir Node.js, npm ni un paso de compilación de frontend para servir estas vistas. Todas las vistas de autenticación MUST reutilizar el lenguaje visual del panel y conservar una experiencia responsiva y accesible.

#### Scenario: Inicio desde un binario distribuido
- **WHEN** el servicio se inicia desde un binario sin directorio de assets frontend
- **THEN** `GET /operations/setup`, `GET /operations/login` y el panel autenticado entregan páginas funcionales que cargan sus dependencias visuales desde CDN

### Requirement: Contrato y documentación del panel
El sistema SHALL incluir las rutas HTML y JSON del panel y de su autenticación en `/openapi.json`, con parámetros, valores por defecto, ejemplos y respuestas de error significativas. Las rutas JSON protegidas MUST declarar el esquema de cookie `operationsSession`; las rutas públicas de setup y login MUST declarar explícitamente que no usan seguridad bearer. El contrato MUST documentar las cookies de sesión y CSRF, la cabecera `X-Operations-CSRF`, las validaciones de credenciales y los errores 400, 401, 403 y 409 aplicables.

README MUST describir la configuración de sesión, el flujo de primera cuenta, que `auth.token` no autoriza el dashboard, el requisito de activar cookies seguras en HTTPS y la dependencia de CDN.

#### Scenario: Especificación OpenAPI del panel autenticado
- **WHEN** un cliente autorizado obtiene `/openapi.json`
- **THEN** encuentra todas las rutas de operaciones con sus esquemas de cookie o ausencia explícita de seguridad, ejemplos y respuestas de error acordes al comportamiento HTTP
