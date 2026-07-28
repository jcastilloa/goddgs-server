## Context

Las rutas actuales llaman a servicios de búsqueda, extracción y research sin un identificador transversal. Research ya calcula diagnósticos, pero éstos no persisten y no cubren fallos internos ni llamadas de modelo. El cambio de almacenamiento aportará el puerto y esquema necesarios.

## Goals / Non-Goals

**Goals:**

- Persistir una operación desde su admisión hasta su resultado final, incluyendo el estado en curso.
- Registrar pasos significativos de búsqueda, extracción y LLM de manera correlacionada y con tiempos.
- Clasificar y sanear errores antes de persistirlos.
- Mantener los contratos HTTP existentes exactamente iguales.

**Non-Goals:**

- Exponer datos mediante HTTP o renderizar el panel.
- Registrar cuerpos de solicitudes/respuestas, credenciales o contenido completo de prompts y resultados de LLM.
- Instrumentar las rutas de operaciones o añadir trazado distribuido externo.

## Decisions

### Contexto de operación explícito

Un middleware de la API creará un ID opaco y la operación raíz para rutas de búsqueda, extracción y research. El ID se propagará mediante `context.Context` hacia los casos de uso y adaptadores. Los servicios fuera del flujo HTTP podrán iniciar operaciones mediante el mismo servicio de aplicación; no se utilizará estado global mutable.

### Registro de inicio antes del trabajo y finalización mediante defer

La raíz se inserta como `running` antes de llamar al handler y se finaliza en un `defer` que captura código HTTP, cancelación, duración y fallo. Los pasos se abren y cierran alrededor de cada llamada externa o fase de orquestación. Así las solicitudes que terminan por error, timeout o cancelación siguen siendo visibles como operaciones completas.

### Modelado de detalle útil y acotado

Cada operación registra tipo, hora, estado, método/ruta, duración y una descripción saneada de la petición. Los pasos registran tipo, proveedor o backend, proxy si existe, horario, duración, estado y metadatos permitidos (por ejemplo query generada, URL sin fragmento, modelo, número de resultados). El tamaño de campos de texto se limitará para evitar crecimiento sin control.

### Clasificación centralizada y saneamiento por defecto

Una única función traduce causas a categorías estables: `canceled`, `timeout`, `rate_limited`, `transport`, `upstream_5xx`, `invalid_response`, `configuration` y `unknown`. Antes de guardar, eliminará credenciales de URL, cabeceras de autorización, claves conocidas y payloads de proveedor. Guardar el error original o usar `Error()` directamente fuera de ese saneador queda prohibido.

### Decoradores en los puertos consumidores

La instrumentación de búsquedas y LLM se añadirá mediante decoradores de los puertos que consumen `search/application`, `research/application` y `shared/extractai/application`. Esto conserva el dominio puro y evita que handlers conozcan los detalles de cada llamada. El middleware sólo posee la operación HTTP raíz.

## Risks / Trade-offs

- [No se propaga el contexto a una llamada interna] → pruebas de correlación para cada tipo de operación y constructores que reciban el registrador explícitamente.
- [El registro añade latencia o falla durante una solicitud] → escrituras cortas; como SQLite es obligatoria, los errores de registro se propagan de manera controlada y se registran localmente sin ocultar el error de negocio.
- [Datos sensibles en queries o URL] → saneador único, retirada de userinfo/fragments y límites de longitud; no se registra body crudo.
- [Demasiados eventos de research] → sólo fases y llamadas significativas, no tokens ni eventos por fragmento.

## Migration Plan

1. Depender de un almacén de operaciones inicializado y añadir tipos, clasificador y saneador con pruebas unitarias.
2. Añadir operación raíz y propagación de contexto a las rutas instrumentadas.
3. Decorar gateways de búsqueda, extracción y LLM, incluyendo research.
4. Ejecutar pruebas de integración con SQLite temporal para éxito, error, timeout y cancelación.

La reversión consiste en eliminar los decoradores y middleware; las filas existentes permanecen y serán eliminadas por la retención.

## Open Questions

- La política exacta para redactar términos de búsqueda sensibles se definirá como una función conservadora al implementar; por defecto se conservará texto acotado porque el usuario solicitó poder consultarlo en el panel.
