## Context

La API no tiene almacenamiento operativo. El binario recibe configuración mediante Viper y su composition root está en `cmd/api/main.go`; los adaptadores viven bajo `platform`. El panel acordado necesita datos que sobrevivan reinicios, durante 30 días, sin una segunda fuente en memoria.

## Goals / Non-Goals

**Goals:**

- Crear, migrar y cerrar una base SQLite obligatoria junto al ejecutable.
- Ofrecer un puerto de operaciones independiente de Gin y SQLite para registrar y consultar datos en cambios posteriores.
- Mantener lecturas de panel y escrituras de peticiones concurrentes con transacciones breves, WAL e índices temporales.
- Eliminar datos vencidos al iniciar y periódicamente durante la ejecución.

**Non-Goals:**

- Instrumentar las solicitudes o servir una interfaz web.
- Autenticación, replicación, copia de seguridad o una base remota.
- Mantener una caché o cola de observabilidad en memoria como vía alternativa.

## Decisions

### SQLite puro de Go, en modo WAL

Se usará un driver SQLite puro de Go para no requerir CGO y se habilitará WAL, una espera acotada ante bloqueos y claves foráneas. SQLite corresponde a la carga local de eventos y consultas por tiempo; DuckDB se descarta porque no aporta valor al patrón OLTP de escrituras cortas y complica la distribución.

### Ubicación resuelta desde el ejecutable

Por defecto el archivo será `operations.sqlite` en el directorio obtenido de `os.Executable`, resolviendo enlaces simbólicos cuando sea posible. Un `operations.database_path` opcional permitirá una sobrescritura explícita. No se usará el directorio de trabajo. El proceso fallará al iniciar si no se puede abrir, migrar o validar la base: no existe un modo degradado en memoria.

### Puerto dirigido por la aplicación y adaptador SQLite

`operations/domain` definirá los tipos inmutables y el puerto mínimo que necesitan los casos de uso. `operations/application` contendrá servicios de registro, consulta y retención; `platform/operations/sqlite` será el único adaptador que conoce SQL. Así los handlers futuros y la instrumentación no importarán `platform`, manteniendo la regla de dependencias del repositorio.

### Esquema orientado a operaciones y eventos

La migración inicial creará `operations`, `operation_steps`, `operation_errors`, `proxy_probes` y `proxy_health_transitions`, con una tabla de versión de esquema. Las columnas temporales se almacenarán en UTC y los índices cubrirán inicio, finalización, estado, tipo y proxy. El cambio de instrumentación poblará las primeras tres tablas; el de sondas poblará las dos últimas.

### Retención consistente de 30 días

La retención se configura como duración, con valor predeterminado `720h`. La limpieza se ejecutará antes de aceptar tráfico y mediante un trabajador con ticker; cada borrado eliminará los registros raíz vencidos y sus hijos por claves foráneas. El trabajador se cancelará y esperará al apagar el proceso.

## Risks / Trade-offs

- [El directorio del binario no es escribible] → fallar temprano con un error que identifique la ruta y documentar el override explícito.
- [Contención de SQLite al mezclar refrescos del panel y eventos] → WAL, transacciones muy breves, consultas paginadas e índices de los filtros publicados.
- [Una migración futura rompe instalaciones existentes] → migraciones versionadas y transaccionales; conservar copias de seguridad fuera del alcance del proceso.
- [La limpieza bloquea operaciones] → borrar por lotes acotados y ejecutar fuera de la ruta de peticiones.

## Migration Plan

1. Añadir la configuración, dependencia y adaptador SQLite con migración inicial.
2. Inicializar el almacén en el composition root antes de crear servicios HTTP; ejecutar limpieza inicial.
3. Iniciar el trabajador de retención con el contexto principal y cerrarlo durante el apagado.
4. Verificar la creación desde una instalación sin base y la apertura de una base existente.

No hay datos previos que migrar. Para revertir una versión de aplicación, detenerla y conservar `operations.sqlite`; la versión anterior la ignorará porque todavía no la abre.

## Open Questions

- Ninguna para esta fase; la frecuencia concreta de limpieza será una constante documentada y podrá hacerse configurable si el uso real lo exige.
