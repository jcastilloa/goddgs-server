## Why

El dashboard de operaciones y toda su API de telemetría son actualmente públicos, aunque exponen datos operacionales sensibles. Es necesario cerrar esa excepción con una identidad local persistida en SQLite, sin reutilizar ni alterar el bearer opcional que protege la API versionada.

## What Changes

- Proteger `GET /operations` y todas las rutas `/operations/api/*` con una sesión propia, opaca y aleatoria, transportada en una cookie HTTP-only; el bearer `auth.token` sigue siendo independiente y no concede acceso al dashboard.
- Añadir una cuenta local de administrador del dashboard persistida en la base SQLite operacional, con nombre de usuario único y contraseña almacenada exclusivamente como hash seguro.
- Entregar una pantalla inicial de creación de cuenta si todavía no existe ningún usuario y una pantalla de inicio de sesión cuando la cuenta ya existe. Ambas conservarán el lenguaje visual, responsividad y accesibilidad del dashboard actual.
- Añadir endpoints de bootstrap, alta inicial, login, sesión actual, logout y cambio de contraseña; las peticiones JSON de telemetría existentes requerirán la misma sesión.
- Incorporar en el dashboard autenticado un indicador del usuario de sesión, cierre de sesión y acceso a cambio de contraseña.
- Actualizar OpenAPI, sus pruebas de contrato y el README para describir las rutas, cookies, validaciones, errores y el aislamiento respecto a `auth.token`.

## Capabilities

### New Capabilities

- `operations-dashboard-authentication`: Identidad local, sesiones y experiencia de acceso para proteger el dashboard de operaciones y su API.

### Modified Capabilities

- `operations-dashboard`: El HTML y la API de consulta dejan de ser públicos y pasan a exigir la sesión específica del dashboard.

## Impact

- Afecta la migración y el adaptador SQLite de operaciones, el nuevo dominio y casos de uso de autenticación, composición en `cmd/api`, DI, handlers y rutas de operaciones, middleware del servidor y los assets HTML embebidos.
- Añade una dependencia criptográfica de hash de contraseñas; los identificadores de sesión aleatorios se almacenan exclusivamente como hash en SQLite y su duración y atributos de cookie se configuran mediante Viper.
- Cambia el contrato de `/operations`, `/operations/api/*`, `/openapi.json`, Swagger y README; los consumidores sin sesión recibirán respuestas de autenticación o serán redirigidos al acceso según el tipo de recurso.
