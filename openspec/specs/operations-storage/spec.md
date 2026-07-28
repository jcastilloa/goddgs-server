## ADDED Requirements

### Requirement: Base de datos de operaciones junto al binario
El sistema SHALL resolver por defecto `operations.sqlite` en el directorio del ejecutable, crearla si no existe y aplicar las migraciones necesarias antes de aceptar tráfico. El sistema MUST permitir una ruta alternativa configurada explícitamente y MUST abortar el arranque si el almacenamiento no se puede inicializar.

#### Scenario: Primera ejecución con directorio escribible
- **WHEN** el ejecutable arranca sin una base de datos de operaciones existente
- **THEN** el sistema crea y migra `operations.sqlite` junto al ejecutable antes de iniciar el servidor HTTP

#### Scenario: Directorio del binario no escribible
- **WHEN** no se puede crear o abrir la base en la ruta resuelta
- **THEN** el proceso no inicia el servidor y devuelve un error que identifica la ruta de almacenamiento

#### Scenario: Ruta explícita de base de datos
- **WHEN** se configura `operations.database_path`
- **THEN** el sistema usa esa ruta en lugar de la ruta predeterminada junto al ejecutable

### Requirement: Esquema persistente de datos operativos
El sistema SHALL mantener un esquema versionado que soporte operaciones, pasos, errores, resultados de sondas y transiciones de salud. Las relaciones dependientes MUST eliminarse al eliminar su registro raíz y las consultas temporales MUST estar indexadas para los filtros del panel.

#### Scenario: Migración de una base existente
- **WHEN** el proceso abre una base con una versión de esquema anterior soportada
- **THEN** aplica las migraciones pendientes de forma transaccional y deja el esquema listo para lectura y escritura

### Requirement: Retención de datos operativos
El sistema SHALL conservar datos operativos durante 30 días por defecto y MUST eliminar los registros vencidos al arrancar y periódicamente mientras se ejecuta. La limpieza MUST eliminar los datos hijos asociados y no bloquear indefinidamente las operaciones normales.

#### Scenario: Limpieza al arrancar
- **WHEN** la base contiene una operación finalizada hace más de la duración de retención
- **THEN** la limpieza inicial elimina la operación y sus registros dependientes antes de que el servidor acepte tráfico

#### Scenario: Operación dentro del periodo de retención
- **WHEN** una operación finalizó dentro de los 30 días configurados
- **THEN** la limpieza no elimina esa operación

### Requirement: Fuente única de datos
El sistema SHALL usar SQLite como fuente única de los datos operativos persistidos. El sistema MUST NOT iniciar una caché, cola o histórico alternativo en memoria como respaldo de observabilidad.

#### Scenario: Error de inicialización del almacenamiento
- **WHEN** el almacenamiento SQLite no se puede inicializar
- **THEN** el proceso falla al arrancar en lugar de continuar con datos operativos sólo en memoria
