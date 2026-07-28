## Why

Las solicitudes de búsqueda, extracción y research, así como sus llamadas a LLM, no producen una traza operativa unificada. Sin esa traza no es posible conocer qué está en curso, analizar latencias, separar éxitos de fallos ni diagnosticar errores de proveedores.

## What Changes

- Registrar el ciclo de vida de las operaciones HTTP de búsqueda, extracción y research en el almacenamiento de operaciones.
- Registrar los pasos internos relevantes, incluidas consultas generadas, búsquedas, extracciones y llamadas LLM, con duración, resultado y error saneado.
- Clasificar errores de búsqueda y LLM de forma estable sin guardar secretos, cabeceras, prompts completos ni respuestas completas.
- Mantener las rutas y respuestas existentes sin cambios funcionales.

## Capabilities

### New Capabilities

- `operation-event-recording`: Registro persistente y consultable de operaciones, pasos y errores saneados.

### Modified Capabilities

- Ninguna.

## Impact

- Depende de `add-operations-storage`.
- Afecta middleware HTTP, casos de uso de búsqueda/research y adaptadores de goddgs/OpenAI.
- Añade pruebas de ciclo de vida, clasificación y saneamiento; no añade todavía endpoints ni interfaz de panel.
