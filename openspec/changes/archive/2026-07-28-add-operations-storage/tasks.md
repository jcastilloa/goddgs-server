## 1. Configuración y ciclo de vida

- [x] 1.1 Añadir la configuración `operations` con ruta opcional de base y retención de 30 días, con validación y ejemplos en `config.sample.yaml`.
- [x] 1.2 Implementar la resolución de ruta predeterminada desde `os.Executable`, incluida la normalización segura de enlaces simbólicos.
- [x] 1.3 Añadir el driver SQLite puro de Go y la apertura obligatoria con WAL, claves foráneas y política acotada de espera ante bloqueo.
- [x] 1.4 Integrar apertura, limpieza inicial, cierre y propagación de errores del almacén en `cmd/api/main.go`.

## 2. Modelo y adaptador de almacenamiento

- [x] 2.1 Definir tipos de dominio y puertos de aplicación para operaciones, pasos, errores, sondas, transiciones y consultas temporales.
- [x] 2.2 Implementar migraciones transaccionales versionadas e índices para operaciones y datos de proxy en el adaptador SQLite.
- [x] 2.3 Implementar el repositorio SQLite con escrituras y consultas básicas necesarias para los cambios dependientes.
- [x] 2.4 Implementar limpieza de registros vencidos, con borrado de dependencias y ejecución periódica cancelable.

## 3. Pruebas y documentación

- [x] 3.1 Escribir primero pruebas de configuración y resolución de ruta, incluidas rutas explícitas y directorios no escribibles.
- [x] 3.2 Escribir pruebas de integración SQLite para primera migración, reapertura, claves foráneas, índices y limpieza de retención.
- [x] 3.3 Verificar el apagado del trabajador de retención sin goroutines pendientes.
- [x] 3.4 Actualizar README con ubicación predeterminada, archivos WAL/SHM, requisito de permisos y override de base.
- [x] 3.5 Ejecutar `gofmt`, `go test ./...`, `go test -race ./...` y `go vet ./...`.
