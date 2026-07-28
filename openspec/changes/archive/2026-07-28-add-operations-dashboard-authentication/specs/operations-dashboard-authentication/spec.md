## ADDED Requirements

### Requirement: Cuenta administradora única y configuración inicial atómica
El sistema SHALL persistir una única cuenta administradora del dashboard en la base SQLite de operaciones. La cuenta MUST almacenar un nombre de usuario único y el hash de su contraseña, y MUST NOT almacenar ni registrar la contraseña en claro. La creación inicial MUST ser atómica: cuando aún no exista cuenta, una sola solicitud concurrente podrá crearla; las restantes MUST recibir un conflicto sin reemplazar la cuenta creada.

El nombre de usuario MUST tener entre 3 y 64 caracteres y contener únicamente letras ASCII, dígitos, punto, guion bajo o guion. La contraseña MUST tener entre 12 y 128 caracteres. El sistema MUST eliminar espacios al inicio y final de ambos campos antes de validarlos.

#### Scenario: Primera creación correcta
- **WHEN** no existe una cuenta y el cliente envía `POST /operations/api/auth/setup` con un nombre de usuario y contraseña válidos
- **THEN** el sistema crea la única cuenta, responde 201, inicia una sesión para ella y no incluye la contraseña ni su hash en la respuesta

#### Scenario: Creación inicial concurrente
- **WHEN** dos solicitudes válidas de configuración inicial alcanzan el servicio cuando no existe una cuenta
- **THEN** exactamente una crea la cuenta y la otra responde 409 con el error documentado `setup_completed`

#### Scenario: Datos de configuración no válidos
- **WHEN** el cliente envía al endpoint de configuración un nombre de usuario o contraseña fuera de la política
- **THEN** el sistema responde 400 con un error de validación documentado y no crea ninguna cuenta

### Requirement: Contraseñas almacenadas y verificadas de forma segura
El sistema SHALL codificar las contraseñas con Argon2id y una sal criptográficamente aleatoria antes de persistirlas. La codificación MUST incluir los parámetros necesarios para verificarla y la verificación MUST usar una comparación resistente a tiempo. El adaptador de persistencia MUST almacenar sólo la codificación de la contraseña, nunca el valor original.

#### Scenario: Base de datos inspeccionada tras crear una cuenta
- **WHEN** una cuenta se ha creado correctamente
- **THEN** su fila de SQLite contiene una codificación Argon2id de la contraseña y no contiene la contraseña proporcionada por el usuario

#### Scenario: Credenciales incorrectas
- **WHEN** un cliente intenta iniciar sesión con un nombre de usuario inexistente o una contraseña incorrecta
- **THEN** el sistema responde 401 con el mismo error genérico documentado y no crea una sesión

### Requirement: Sesiones opacas persistidas y revocables
El sistema SHALL crear sesiones mediante un identificador aleatorio criptográficamente seguro, enviado sólo en la cookie `operations_session` y persistido únicamente como hash. Cada sesión MUST estar asociada a la cuenta administradora y tener una fecha de caducidad. Las sesiones vencidas o revocadas MUST dejar de autorizar solicitudes inmediatamente.

La cookie `operations_session` MUST usar `HttpOnly`, `Path=/operations`, `SameSite=Strict` y el atributo `Secure` definido por la configuración. El sistema MUST enviar además la cookie legible `operations_csrf` con un valor aleatorio por sesión, el mismo `Path`, `SameSite=Strict` y el atributo `Secure` configurado. El sistema MUST borrar ambas cookies al detectar una sesión inválida, al cerrar sesión y al sustituir una sesión por cambio de contraseña.

#### Scenario: Inicio de sesión correcto
- **WHEN** una cuenta existente envía credenciales válidas a `POST /operations/api/auth/login`
- **THEN** el sistema responde 200, establece las cookies de sesión y CSRF, y devuelve únicamente el nombre de usuario de la sesión

#### Scenario: Sesión vencida en una API protegida
- **WHEN** un cliente solicita una ruta JSON protegida con una cookie de sesión vencida
- **THEN** el sistema borra las cookies de autenticación y responde 401 con `{"error":"dashboard authentication required"}`

#### Scenario: Sesión revocada
- **WHEN** la fila de una sesión ha sido revocada o eliminada antes de una solicitud posterior
- **THEN** esa solicitud no queda autorizada aunque conserve el valor de cookie anterior

### Requirement: Protección CSRF para mutaciones autenticadas
El sistema SHALL exigir la cabecera `X-Operations-CSRF` en las solicitudes autenticadas que cambian estado. El valor MUST coincidir con la cookie `operations_csrf` asociada a la sesión. Esta comprobación MUST aplicarse como mínimo a `POST /operations/api/auth/logout` y `POST /operations/api/auth/password`; los formularios de setup y login no dependerán de una sesión existente y no requerirán esta cabecera.

#### Scenario: Cierre de sesión con CSRF válido
- **WHEN** una sesión autenticada envía `POST /operations/api/auth/logout` con la cabecera CSRF que coincide con su cookie
- **THEN** el sistema revoca la sesión, borra las cookies y responde 204

#### Scenario: Mutación sin CSRF válido
- **WHEN** una sesión autenticada envía logout o cambio de contraseña sin la cabecera CSRF correcta
- **THEN** el sistema responde 403 y conserva la sesión existente

### Requirement: Ciclo de vida de sesión y credenciales del dashboard
El sistema SHALL ofrecer `GET /operations/api/auth/session` para identificar la sesión autenticada, `POST /operations/api/auth/logout` para cerrarla y `POST /operations/api/auth/password` para cambiar la contraseña. El endpoint de sesión MUST responder 200 con `{"username":"..."}` para una sesión válida y 401 para otra situación.

El cambio de contraseña MUST exigir `current_password` y `new_password`; la contraseña actual MUST verificarse y la nueva MUST cumplir la política y ser distinta de la actual. Tras un cambio correcto, el sistema MUST invalidar todas las sesiones previas de la cuenta y crear una sesión nueva para la solicitud actual, con un nuevo valor CSRF. El endpoint MUST responder 204 y establecer las nuevas cookies.

#### Scenario: Consulta de identidad de sesión
- **WHEN** una sesión válida solicita `GET /operations/api/auth/session`
- **THEN** recibe 200 con su nombre de usuario y sin datos de contraseña, hash o token

#### Scenario: Cambio de contraseña correcto
- **WHEN** una sesión autenticada envía la contraseña actual correcta, una nueva contraseña válida distinta y un CSRF válido
- **THEN** el sistema actualiza el hash, revoca las sesiones anteriores y devuelve una nueva sesión autenticada para esa solicitud

#### Scenario: Contraseña actual incorrecta
- **WHEN** una sesión autenticada intenta cambiar la contraseña con `current_password` incorrecta
- **THEN** el sistema responde 401, conserva la contraseña y no revoca la sesión actual

### Requirement: Flujo HTML de configuración y acceso
El sistema SHALL servir `GET /operations/setup` como pantalla de configuración cuando todavía no exista una cuenta y `GET /operations/login` como pantalla de acceso cuando ya exista. Una sesión válida que solicite cualquiera de esas rutas MUST recibir una redirección 303 a `/operations`. Cuando existe una cuenta, `GET /operations/setup` MUST redirigir 303 a `/operations/login`; cuando no existe, `GET /operations/login` MUST redirigir 303 a `/operations/setup`.

Las pantallas de configuración, acceso y cambio de contraseña MUST mantener el lenguaje visual del dashboard: superficies oscuras, tipografía y paleta consistentes, jerarquía clara, estados de foco visibles, mensajes de validación accesibles y disposición utilizable en móvil y escritorio. Los formularios MUST evitar registrar o mostrar contraseñas y comunicar los errores de las respuestas JSON sin recargar innecesariamente la página.

#### Scenario: Primera visita sin cuenta
- **WHEN** no existe una cuenta y un usuario solicita `GET /operations`
- **THEN** recibe una redirección 303 a `/operations/setup`, donde puede crear las credenciales iniciales

#### Scenario: Visita anónima después de la configuración
- **WHEN** ya existe una cuenta y un usuario sin sesión solicita `GET /operations`
- **THEN** recibe una redirección 303 a `/operations/login`

### Requirement: Aislamiento respecto a la autenticación bearer existente
La autenticación del dashboard SHALL ser independiente de `auth.token`. Una cabecera bearer válida o inválida MUST NOT crear ni sustituir una sesión de dashboard ni conceder acceso a sus rutas protegidas. Las rutas `/v1`, `/openapi.json` y `/docs` MUST conservar el comportamiento de autenticación bearer existente.

#### Scenario: Bearer sin sesión de dashboard
- **WHEN** `auth.token` está configurado y un cliente solicita `GET /operations` con un bearer válido pero sin cookie de sesión de dashboard
- **THEN** el servidor redirige al flujo de setup o login y no entrega el dashboard

#### Scenario: API versionada sin cambios
- **WHEN** `auth.token` está configurado y un cliente solicita una ruta `/v1` sin bearer
- **THEN** el servidor continúa respondiendo 401 independientemente del estado de la sesión de dashboard

### Requirement: Configuración de seguridad de sesión
El sistema SHALL obtener mediante Viper `operations.dashboard_auth.session_ttl` y `operations.dashboard_auth.cookie_secure`. El valor de `session_ttl` MUST ser una duración positiva y, si no se configura, MUST ser 12 horas. `cookie_secure` MUST ser `false` por defecto para el uso HTTP local y MUST poder activarse para despliegues HTTPS.

#### Scenario: Duración de sesión predeterminada
- **WHEN** no se define `operations.dashboard_auth.session_ttl`
- **THEN** una sesión nueva expira 12 horas después de su creación

#### Scenario: Cookie segura en HTTPS
- **WHEN** se configura `operations.dashboard_auth.cookie_secure=true` y se inicia una sesión
- **THEN** las cookies de sesión y CSRF incluyen el atributo `Secure`
