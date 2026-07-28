## ADDED Requirements

### Requirement: Configuración explícita de sondas
El sistema SHALL permitir configurar una sonda de proxy con URL, intervalo, timeout y umbrales de éxito y fallo consecutivos. Cuando las sondas estén habilitadas, la URL MUST ser HTTP(S), el intervalo y timeout MUST ser positivos y ambos umbrales MUST ser positivos.

#### Scenario: Configuración válida de sonda
- **WHEN** el operador configura una URL HTTPS, intervalos positivos y umbrales positivos
- **THEN** el servicio inicia la supervisión de sondas después de inicializar los proxies

#### Scenario: Configuración de sonda inválida
- **WHEN** las sondas están habilitadas con una URL no HTTP(S) o un timeout no positivo
- **THEN** el proceso no inicia y devuelve un error de configuración

### Requirement: Sondas activas por proxy
El sistema SHALL ejecutar periódicamente una petición HTTP mínima a través de cada proxy configurado, incluyendo proxies respaldados por túneles SSH. La sonda MUST registrar duración, hora, estado HTTP cuando exista y resultado; sólo una respuesta 2xx o 3xx constituye éxito.

#### Scenario: Salida satisfactoria por proxy
- **WHEN** la URL de sonda responde 204 a través de un proxy
- **THEN** el sistema persiste una sonda exitosa con su duración y código HTTP

#### Scenario: Timeout de sonda
- **WHEN** una sonda excede su timeout configurado
- **THEN** el sistema persiste una sonda fallida clasificada como timeout y continúa con las siguientes rondas

### Requirement: Estado de salud con histéresis
El sistema SHALL iniciar cada proxy en estado `unknown`, actualizarlo usando los umbrales configurados y persistir cada transición. El sistema MUST marcar `degraded` tras un fallo que no alcance el umbral de caída y `unhealthy` al alcanzar dicho umbral; sólo MUST marcar `healthy` tras alcanzar el umbral de éxitos.

#### Scenario: Fallos consecutivos suficientes
- **WHEN** un proxy alcanza el umbral configurado de fallos consecutivos
- **THEN** el sistema lo marca `unhealthy`, persiste una transición y deja de seleccionarlo en el pool

#### Scenario: Recuperación confirmada
- **WHEN** un proxy no saludable alcanza el umbral configurado de éxitos consecutivos
- **THEN** el sistema lo marca `healthy`, persiste la transición y permite que el pool lo seleccione

### Requirement: Señal del túnel SSH
El sistema SHALL considerar la desconexión de un túnel SSH una condición inmediata de estado `unhealthy`. Cuando el túnel vuelva a conectar, el sistema MUST volver el estado a `unknown` hasta que una sonda confirme una salida satisfactoria.

#### Scenario: Túnel SSH desconectado
- **WHEN** el túnel que respalda un proxy SSH informa desconexión
- **THEN** el proxy se marca inmediatamente `unhealthy` y no se selecciona para tráfico

### Requirement: Ciclo de vida del supervisor
El sistema SHALL iniciar el supervisor de sondas con el ciclo de vida de la aplicación y MUST cancelar sus rondas y esperar su finalización durante el apagado.

#### Scenario: Apagado durante una sonda
- **WHEN** el proceso recibe una señal de apagado mientras una sonda está en curso
- **THEN** la petición de sonda recibe cancelación de contexto y el proceso no deja goroutines de supervisor activas
