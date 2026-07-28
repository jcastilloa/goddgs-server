## 1. Dominio, seguridad y persistencia

- [x] 1.1 Añadir las dependencias criptográficas necesarias y definir en `operations/domain` los tipos, errores y puertos mínimos para cuenta de dashboard y sesión revocable.
- [x] 1.2 Implementar y probar de forma aislada la política de nombre de usuario/contraseña, hash y verificación Argon2id, generación criptográfica de tokens y hash de token sin exponer secretos.
- [x] 1.3 Crear la migración SQLite transaccional para `operations_dashboard_users` y `operations_dashboard_sessions`, con las restricciones e índices de búsqueda/caducidad requeridos.
- [x] 1.4 Implementar y probar el adaptador SQLite para consultar/crear atómicamente la cuenta, crear/validar/revocar sesiones y eliminar sesiones expiradas.

## 2. Casos de uso y configuración

- [x] 2.1 Implementar y probar los casos de uso de estado de bootstrap, alta inicial, login, consulta de sesión, logout y cambio de contraseña, incluidas las condiciones de carrera y la revocación total de sesiones.
- [x] 2.2 Añadir `operations.dashboard_auth.session_ttl` y `operations.dashboard_auth.cookie_secure` a la configuración Viper, valores por defecto, validación y `config.sample.yaml`.
- [x] 2.3 Componer el servicio de autenticación con el store SQLite existente en `cmd/api/main.go` y registrar sus dependencias y labels en el contenedor DI sin introducir importaciones de `platform` en `operations`.

## 3. HTTP, middleware y rutas

- [x] 3.1 Implementar handlers de setup, login, sesión, logout y cambio de contraseña con validación de JSON, mapeo estable de errores y cookies de sesión/CSRF seguras.
- [x] 3.2 Implementar middleware de dashboard que distingue HTML de JSON, valida sesiones, limpia cookies inválidas y dirige a setup/login con 303 o responde 401 según corresponda.
- [x] 3.3 Aplicar la comprobación CSRF a logout y cambio de contraseña; verificar que las solicitudes rechazadas no revocan ni cambian credenciales.
- [x] 3.4 Registrar las rutas públicas y protegidas de `/operations` y `/operations/api/*`, manteniendo el middleware bearer y las rutas `/v1`, `/openapi.json` y `/docs` sin cambios.
- [x] 3.5 Añadir pruebas HTTP de setup, login, cookie segura, sesión caducada, logout, CSRF, cambio de contraseña, invalidación de sesiones y aislamiento de `auth.token`.

## 4. Experiencia del dashboard

- [x] 4.1 Crear las vistas HTML embebidas de configuración inicial y login usando el mismo sistema visual del dashboard, con formularios responsivos, accesibles y mensajes de error útiles.
- [x] 4.2 Incorporar en el dashboard autenticado el badge de usuario en la esquina superior derecha, controles accesibles de logout y cambio de contraseña y sus estados de carga/error.
- [x] 4.3 Actualizar el JavaScript del dashboard para enviar CSRF en mutaciones y redirigir a login al recibir 401, sin continuar mostrando telemetría protegida.
- [x] 4.4 Añadir pruebas de handlers y assets que cubran las redirecciones de acceso y la presencia de las interacciones de sesión requeridas.

## 5. Contrato, documentación y verificación

- [x] 5.1 Actualizar `platform/server/openapi.go` con el esquema `operationsSession`, las rutas HTML y JSON de autenticación, cookies, cabecera CSRF, validaciones, ejemplos y respuestas 400/401/403/409.
- [x] 5.2 Actualizar `platform/server/openapi_test.go` para verificar el esquema de cookie, la seguridad de cada ruta, parámetros/cuerpos, ejemplos y respuestas no satisfactorias del dashboard autenticado.
- [x] 5.3 Actualizar README con el bootstrap de la única cuenta, login, cierre/cambio de contraseña, configuración de cookie segura para HTTPS, aislamiento de `auth.token` y dependencia CDN.
- [x] 5.4 Ejecutar `gofmt` en el código modificado, pruebas focalizadas de dominio/SQLite/HTTP/OpenAPI y finalmente `go test ./...` y `go vet ./...`.
