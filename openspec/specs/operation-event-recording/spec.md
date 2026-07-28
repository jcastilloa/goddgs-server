## ADDED Requirements

## Purpose

Definir el registro correlacionado, saneado y observable de operaciones y sus
pasos.
## Requirements
### Requirement: Ciclo de vida de operaciones HTTP
El sistema SHALL registrar una operación para cada solicitud de búsqueda, extracción o research antes de ejecutar su trabajo y SHALL finalizarla con estado, hora de finalización, duración y resultado HTTP. Las operaciones en curso MUST permanecer consultables como `running` hasta que finalicen.

#### Scenario: Solicitud de búsqueda en curso
- **WHEN** el servidor acepta una solicitud de búsqueda instrumentada y su caso de uso sigue ejecutándose
- **THEN** el almacenamiento contiene una operación `running` con un identificador opaco, tipo de operación y hora de inicio

#### Scenario: Solicitud completada correctamente
- **WHEN** una solicitud instrumentada devuelve una respuesta de éxito
- **THEN** su operación se marca como exitosa con duración y código HTTP final

#### Scenario: Solicitud cancelada o con timeout
- **WHEN** el contexto de una solicitud instrumentada se cancela o expira
- **THEN** su operación se finaliza con el resultado correspondiente y un error clasificado

### Requirement: Registro correlacionado de pasos
El sistema SHALL registrar los pasos significativos de búsqueda, extracción, planificación LLM y generación de informe asociados a su operación raíz. Cada paso MUST almacenar su tipo, estado, duración, proveedor o backend cuando exista y proxy cuando se conozca.

#### Scenario: Research con búsquedas y LLM
- **WHEN** una operación de research genera consultas, busca fuentes, extrae contenido y genera un informe
- **THEN** el almacenamiento conserva pasos correlacionados para cada fase completada con sus duraciones

#### Scenario: Fallo de una llamada de proveedor
- **WHEN** una búsqueda o llamada LLM falla
- **THEN** el paso correspondiente se marca como fallido y queda asociado a la operación raíz sin alterar el contrato HTTP existente

### Requirement: Errores saneados y clasificados
El sistema SHALL persistir los errores de búsquedas, extracciones y LLM con una categoría estable y un mensaje saneado. El sistema MUST NOT persistir tokens, claves, cabeceras de autorización, cuerpos completos ni prompts o respuestas completas de LLM.

#### Scenario: Error de proveedor con credenciales
- **WHEN** un error incluye una URL con userinfo, un header de autorización o una clave de proveedor
- **THEN** el error almacenado no contiene esos valores secretos

#### Scenario: Límite de tasa
- **WHEN** un proveedor de búsqueda o LLM devuelve un error de rate limit
- **THEN** el error almacenado se clasifica como `rate_limited`

### Requirement: Compatibilidad del API existente
El sistema SHALL mantener sin cambios las rutas, solicitudes, respuestas, códigos de estado y autenticación existentes de búsqueda, extracción y research al registrar eventos operativos.

#### Scenario: Solicitud válida existente
- **WHEN** un cliente llama a una ruta API existente con los mismos parámetros que antes de la instrumentación
- **THEN** recibe la misma forma de respuesta y código HTTP que recibiría sin el registro operativo

### Requirement: Registro de selección de fuentes de research
El sistema SHALL registrar un paso `research_selection` correlacionado con cada operación de research que alcance la selección previa a extracción. El paso MUST guardar su estado y duración junto con el número de candidatos descubiertos y seleccionados, y MUST NOT persistir URLs, títulos, descripciones, prompts ni respuestas completas del LLM.

#### Scenario: Selección previa completada
- **WHEN** una operación de research entrega candidatos de búsqueda al selector y recibe una selección válida
- **THEN** el almacenamiento conserva un paso `research_selection` completado con su duración y ambos conteos, asociado a la operación raíz

#### Scenario: Fallo del selector
- **WHEN** la llamada al LLM selector falla o devuelve una selección inválida
- **THEN** el paso `research_selection` se marca como fallido con un error saneado y no persiste metadatos de candidatos ni contenido del LLM
