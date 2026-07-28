## Context

El cambio `add-operations-dashboard` dejó intencionadamente `GET /operations` y `/operations/api/*` fuera del middleware de bearer de la API. El dashboard ya se entrega como HTML embebido, consulta una única base SQLite operacional y sus datos están saneados, pero siguen siendo sensibles. La aplicación ya abre esa base antes de construir DI y dispone de un sistema de migraciones transaccionales versionadas.

La nueva autenticación debe ser local al dashboard: no puede depender de `auth.token`, no puede modificar el comportamiento de `/v1`, `/openapi.json` ni `/docs`, y debe conservar el diseño oscuro, responsivo y sin build frontend del dashboard. En el primer acceso no hay credenciales preconfiguradas: el primer operador crea la cuenta que quedará persistida.

## Goals / Non-Goals

**Goals:**

- Requerir una sesión de dashboard válida antes de servir telemetría HTML o JSON.
- Permitir de forma segura y atómica la creación de la única cuenta administradora cuando el almacén aún no contiene usuarios.
- Almacenar contraseñas y sesiones con defensas adecuadas para un servicio HTTP local o tras proxy inverso.
- Ofrecer pantallas de configuración inicial, login y cambio de contraseña que compartan el lenguaje visual y la calidad de interacción del dashboard existente.
- Permitir cierre de sesión e invalidar las sesiones activas al cambiar la contraseña.
- Documentar y probar todos los contratos HTTP y preservar las fronteras de capas `platform -> operations`.

**Non-Goals:**

- Soportar múltiples usuarios, roles, recuperación de contraseña, SSO/OAuth, MFA ni gestión administrativa de usuarios.
- Reutilizar, aceptar o sustituir `auth.token` como credencial del dashboard.
- Proteger `/v1`, `/openapi.json` o `/docs` con esta nueva sesión, ni cambiar su autorización actual.
- Introducir Node.js, una SPA compilada, un proveedor externo de identidad ni sesiones en memoria.

## Decisions

### Una sola cuenta persistida y creación inicial atómica

Se añadirá una migración SQLite con una tabla `operations_dashboard_users` de una sola fila lógica: `id`, `username` único, `password_hash`, `created_at` y `updated_at`. El caso de uso expondrá el estado de bootstrap sin revelar identidades y creará la cuenta mediante inserción condicional en una transacción. Si dos primeras visitas compiten, sólo una podrá crearla; la otra recibirá `409 setup_completed` y deberá iniciar sesión.

El dominio de operaciones definirá el agregado mínimo `DashboardUser` y puertos pequeños para consultar/crear el usuario, verificar credenciales, actualizar el hash y revocar sesiones. La aplicación orquestará validación, hash/verificación y revocación; el adaptador SQLite será el único que importe `database/sql`.

Una tabla separada `operations_dashboard_sessions` almacenará `token_hash`, `user_id`, `expires_at` y `created_at`, indexada por hash y caducidad. El identificador de sesión aleatorio nunca se persiste ni se registra en claro. Se usará un generador criptográfico y SHA-256 para la búsqueda del token; el valor se comparará en tiempo constante. Esta sesión stateful se elige frente a JWT porque permite logout e invalidación inmediata de todas las sesiones al cambiar la contraseña, sin clave JWT adicional ni lista de revocación.

### Contraseñas mediante Argon2id y política explícita

Las contraseñas se almacenarán como codificaciones autocontenidas Argon2id con sal aleatoria y parámetros definidos en código. La política inicial exigirá usuario de 3–64 caracteres (letras, dígitos, punto, guion bajo y guion) y contraseña de 12–128 caracteres; los handlers normalizan espacios externos pero nunca registran ni devuelven las contraseñas. Una implementación mantenida de `golang.org/x/crypto/argon2` realizará el hash y la verificación constante.

Se prefiere Argon2id sobre bcrypt porque es resistente a ataques modernos de GPU y no introduce dependencia de configuración. La política se validará tanto en dominio/aplicación como en los contratos HTTP. El cambio de contraseña requerirá la contraseña actual y una contraseña nueva válida y distinta; al completarse, borrará todas las sesiones del usuario y emitirá una nueva para que la sesión actual continúe de forma predecible.

### Cookie de sesión y protección CSRF de rutas mutantes

Una sesión autenticada se transportará mediante la cookie `operations_session`, con `HttpOnly`, `Path=/operations`, `SameSite=Strict` y `Secure` configurable para admitir desarrollo HTTP local y producción HTTPS. Su vida se configura como `operations.dashboard_auth.session_ttl`, con un valor predeterminado de 12 horas y validación positiva. No habrá token en `localStorage`, en fragmentos URL ni en respuestas JSON.

La cookie se acompañará de una cookie no HTTP-only `operations_csrf`, con un secreto aleatorio por sesión. `POST /operations/api/auth/logout` y `POST /operations/api/auth/password` exigirán el valor coincidente en la cabecera `X-Operations-CSRF`; el JavaScript embebido lo enviará automáticamente. La alta inicial y el login no necesitan CSRF porque no dependen de una sesión autenticada. Se elige doble envío en cookie frente a token HTML porque no requiere renderizado dinámico y protege las mutaciones autenticadas incluso tras una integración futura con proxy.

Al desautenticar, expirar, faltar o ser inválida la sesión, `GET /operations` redirige con `303` a `/operations/login`; las rutas JSON protegidas devuelven `401` con `{"error":"dashboard authentication required"}`. Una cookie válida para un usuario eliminado o una sesión expirada se borra y sigue esa misma semántica. Las rutas de login, bootstrap y sus POST son las únicas excepciones explícitas del middleware del dashboard.

### Rutas y experiencia de acceso consistente con el dashboard

El grupo `/operations` se divide explícitamente entre rutas públicas de acceso y rutas protegidas. Las nuevas rutas serán:

- `GET /operations/setup` y `POST /operations/api/auth/setup` para comprobar y completar la primera configuración.
- `GET /operations/login` y `POST /operations/api/auth/login` para acceso posterior.
- `GET /operations/api/auth/session` para obtener el nombre de usuario de la sesión actual.
- `POST /operations/api/auth/logout` para cerrar sesión.
- `POST /operations/api/auth/password` para cambiar la contraseña.

`GET /operations` seguirá siendo la URL canónica: el middleware redirigirá a setup o login según corresponda y sólo devolverá el dashboard a una sesión válida. `GET /operations/setup` y `GET /operations/login` redirigirán a `/operations` para una sesión válida; setup también redirigirá al login si ya existe usuario. Las respuestas de formulario se entregarán como HTML embebido y los POST devolverán JSON para mantener interacciones rápidas y mensajes accesibles sin duplicar páginas.

Los assets de setup y login reutilizarán los colores, tipografía, marca, superficies, controles, estados de foco y diseño responsivo del dashboard. El dashboard autenticado añadirá en la esquina superior derecha un badge con el nombre del usuario y estado de sesión, un menú o panel accesible con "Cambiar contraseña" y "Cerrar sesión", y estados de carga/error claros. Las llamadas `fetch` gestionarán 401 redirigiendo al login y adjuntarán el CSRF en mutaciones.

### Middleware, DI y capas

Los handlers HTTP permanecerán en `platform/handlers/operations`, con labels y registro en `platform/di/container.go`; `platform/routes/operations.go` registrará rutas públicas y protegidas de forma legible. `platform/server` recibirá un verificador de sesión para construir el middleware y decidir redirecciones/JSON, sin que `operations` importe Gin, SQLite o `platform`.

Los tipos de usuario, sesión, errores de credencial y puertos viven en `operations/domain`; los casos de uso en `operations/application`; el store existente implementa los puertos desde `platform/operations/sqlite`. `cmd/api/main.go` compone el servicio de autenticación con el mismo store SQLite que ya posee el dashboard antes de crear el contenedor. Las interfaces se definirán junto al consumidor y sólo para los límites de persistencia/seguridad necesarios.

### Configuración y contrato público

Viper leerá bajo `operations.dashboard_auth`: `session_ttl` y `cookie_secure`, con valores por defecto de `12h` y `false`; el sample config y README indicarán que `cookie_secure` debe ser `true` cuando el navegador llega por HTTPS. Los parámetros Argon2 son deliberadamente internos para no convertirlos en configuración operativa insegura.

OpenAPI conservará la seguridad global bearer para API, documentación y Swagger, pero las operaciones del dashboard declararán un esquema de cookie `operationsSession` en sus rutas protegidas en lugar de `security: []`. Las rutas públicas de bootstrap/login declararán `security: []`; todos los endpoints reflejarán `401`, `403`, `409`, validaciones y cabeceras/cookies relevantes. `/openapi.json` sigue protegido sólo por `auth.token` cuando éste existe.

## Risks / Trade-offs

- [Primera persona que alcance setup puede adueñarse del dashboard] → la creación se limita a una sola transacción, queda documentado como requisito de despliegue y los operadores deben iniciar el servicio en una red controlada hasta completar la configuración inicial.
- [Una sesión stateful añade escrituras e índices SQLite] → son pocas y acotadas; se usan índices por hash/caducidad y una limpieza de sesiones expiradas al abrir y de forma oportunista en las operaciones de autenticación.
- [Argon2id consume CPU y memoria] → los parámetros se seleccionarán para un servidor pequeño y se probará que los handlers mantienen límites de tamaño y no permiten contraseñas arbitrariamente largas.
- [Cookies requieren HTTPS en producción] → `cookie_secure` está expuesto, documentado y probado; `SameSite=Strict`, `HttpOnly` y CSRF cubren el resto del navegador.
- [Redirecciones HTML y errores JSON pueden divergir] → un único middleware clasificará por ruta, y las pruebas HTTP cubrirán explícitamente ambos comportamientos.
- [Cambiar de dashboard público a privado rompe scripts existentes] → es un cambio intencional de seguridad, descrito en README y OpenAPI; los scripts deberán mantener cookie de sesión o acceder con un cliente que soporte el flujo de login.

## Migration Plan

1. Añadir migración de usuarios/sesiones, puertos de dominio y pruebas aisladas de repositorio, hash y casos de uso.
2. Añadir configuración, composición, DI, middleware y rutas de autenticación; probar setup atómico, login, expiración, logout, CSRF y cambio de contraseña.
3. Proteger el HTML y las APIs existentes; incorporar las pantallas y el badge/acciones en el dashboard, con pruebas de assets y pruebas HTTP de redirección/401.
4. Actualizar OpenAPI, pruebas de contrato, `config.sample.yaml` y README; ejecutar `go test ./...` y `go vet ./...`.

El despliegue aplica la migración al inicio. Hasta que se complete setup, `/operations` muestra sólo la pantalla de alta inicial; los registros de operaciones existentes no cambian. Para revertir código, restaurar una versión que ignore las tablas nuevas es seguro para los datos de operaciones; las tablas de usuarios y sesiones pueden permanecer inertes. Para revocar acceso tras un incidente, el operador podrá eliminar las filas de sesiones y reiniciar, y un cambio de contraseña invalida todas las sesiones de la cuenta.

## Open Questions

- Ninguna bloqueante. Se adopta inicialmente una única cuenta administradora, 12 horas de sesión y cookie `Secure` desactivada por defecto para que la configuración local HTTP funcione sin opciones adicionales; el README mostrará la configuración HTTPS recomendada.
