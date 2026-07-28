## ADDED Requirements

### Requirement: Rutas separadas del panel de operaciones
El sistema SHALL servir el panel HTML en `GET /operations` y su API JSON bajo `/operations/api/*` en el mismo host y puerto que la API. Estas rutas MUST NOT requerir el bearer token configurado para `/v1` durante esta fase; las rutas API, OpenAPI y Swagger existentes MUST conservar su comportamiento de autenticación.

#### Scenario: Acceso al panel con bearer de API configurado
- **WHEN** `auth.token` está configurado y un cliente solicita `GET /operations` sin cabecera Authorization
- **THEN** el servidor responde el panel HTML sin exigir el bearer token

#### Scenario: API existente con bearer configurado
- **WHEN** `auth.token` está configurado y un cliente solicita una ruta `/v1` sin credenciales
- **THEN** el servidor continúa respondiendo 401

### Requirement: API de consulta de operaciones
El sistema SHALL proporcionar endpoints JSON para resumen, serie temporal, operaciones paginadas, detalle por identificador y proxies. Los endpoints MUST validar fechas, rangos, intervalos y límites, y MUST devolver datos saneados desde el almacenamiento de operaciones.

#### Scenario: Consulta de operaciones recientes
- **WHEN** un cliente solicita `GET /operations/api/operations` con un límite válido
- **THEN** recibe una lista paginada de operaciones con estado, tipo, inicio, duración y resultado

#### Scenario: Parámetro temporal inválido
- **WHEN** un cliente solicita una serie temporal con un rango o intervalo no permitido
- **THEN** el servidor responde 400 con un error documentado

#### Scenario: Operación inexistente
- **WHEN** un cliente solicita una operación que no existe
- **THEN** el servidor responde 404 con un error documentado

### Requirement: Resumen y evolución operacional
El panel SHALL mostrar operaciones activas, totales de éxitos y errores, percentiles p50/p95 de duración y gráficas de volumen, fallos y latencia para los periodos de 24 horas, 7 días y 30 días. El panel MUST refrescar sus datos sin recargar la página.

#### Scenario: Datos de 24 horas disponibles
- **WHEN** existen operaciones finalizadas en las últimas 24 horas
- **THEN** el panel muestra los indicadores y las gráficas agregadas para ese periodo

#### Scenario: Sin operaciones registradas
- **WHEN** no existen operaciones en el rango seleccionado
- **THEN** el panel muestra estados vacíos claros sin errores de JavaScript ni gráficas engañosas

### Requirement: Salud visual de proxies
El panel SHALL mostrar estado actual, última sonda, latencia y evolución de disponibilidad de los proxies cuando existan datos de proxy. El panel MUST ocultar la sección de proxies cuando no haya proxies configurados o no existan datos de sonda.

#### Scenario: Proxies con resultados de sonda
- **WHEN** el almacenamiento contiene estados y resultados de sonda de proxies
- **THEN** el panel muestra una tarjeta por proxy y su evolución de estado y latencia

#### Scenario: Sin proxies configurados
- **WHEN** el servicio no tiene proxies disponibles para mostrar
- **THEN** el panel no muestra una sección vacía de proxies y conserva las secciones generales de operaciones

### Requirement: Entrega sin build frontend
El sistema SHALL entregar el panel con HTML servido por Go, Tailwind CSS desde CDN y Chart.js desde CDN. El proyecto MUST NOT requerir Node.js, npm ni un paso de compilación de frontend para servir el panel.

#### Scenario: Inicio desde un binario distribuido
- **WHEN** el servicio se inicia desde un binario sin directorio de assets frontend
- **THEN** `GET /operations` entrega una página funcional que carga sus dependencias visuales desde CDN

### Requirement: Contrato y documentación del panel
El sistema SHALL incluir las rutas HTML y JSON del panel en `/openapi.json`, con parámetros, valores por defecto, ejemplos y respuestas de error significativas. La documentación MUST expresar que el panel no usa el bearer de `/v1` en esta fase y README MUST describir la configuración, dependencia de CDN y autenticación propia pendiente.

#### Scenario: Especificación OpenAPI del panel
- **WHEN** un cliente obtiene `/openapi.json`
- **THEN** encuentra las rutas de operaciones con contratos de respuesta y sin requisito de `bearerAuth`
