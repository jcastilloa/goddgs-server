## Why

El servicio no conserva un histórico operativo consultable; por tanto no puede mostrar operaciones en curso, resultados, errores ni tendencias más allá de los logs del proceso. Se necesita una base local y duradera como fundamento del futuro panel de operaciones.

## What Changes

- Añadir almacenamiento SQLite obligatorio para datos de operaciones, creado y migrado automáticamente junto al binario ejecutable.
- Conservar registros durante 30 días y eliminarlos de forma periódica y al arrancar.
- Exponer puertos de escritura y consulta que permitan a las siguientes fases registrar operaciones, pasos, errores y resultados de sondas sin depender de Gin ni de detalles de SQLite.
- Hacer que el arranque falle de forma explícita cuando no se pueda inicializar el almacenamiento, en lugar de simular observabilidad sin persistencia.

## Capabilities

### New Capabilities

- `operations-storage`: Persistencia SQLite con ciclo de vida, migraciones, consultas básicas y retención de los datos operativos.

### Modified Capabilities

- Ninguna.

## Impact

- Añade un paquete de dominio/aplicación de operaciones y un adaptador SQLite en `platform`.
- Añade un driver SQLite puro de Go y configuración documentada para la retención y la ubicación opcional de la base.
- Modifica el composition root para iniciar, cerrar y limpiar el almacenamiento de forma segura.
