## 1. Configuración y dependencias de transporte

- [x] 1.1 Añadir y validar `operations.probe` con URL, intervalo, timeout y umbrales, y documentarlo en `config.sample.yaml` y README.
- [x] 1.2 Extender la construcción de gateway para conservar de manera interna la URL de transporte efectiva de cada proxy, incluidos túneles SSH.
- [x] 1.3 Definir puertos para escribir resultados de sonda y transiciones usando el almacenamiento de `add-operations-storage`.

## 2. Sondas y máquina de estados

- [x] 2.1 Escribir primero pruebas table-driven para los estados `unknown`, `healthy`, `degraded` y `unhealthy` y sus umbrales.
- [x] 2.2 Implementar la máquina de estados con contadores consecutivos, persistencia única de transiciones e integración con el pool.
- [x] 2.3 Implementar el cliente de sonda HTTP por proxy con contexto, timeout, clasificación de resultado y cierre correcto del body.
- [x] 2.4 Implementar el supervisor periódico cancelable y su coordinación con inicio/apagado de la aplicación.
- [x] 2.5 Integrar la señal de túnel SSH para forzar caída y exigir una sonda posterior para recuperación.

## 3. Verificación y documentación

- [x] 3.1 Añadir pruebas de integración con transportes HTTP controlados para 2xx/3xx, 4xx/5xx, transporte y timeout.
- [x] 3.2 Añadir pruebas de persistencia de cada resultado, transición y latencia en SQLite temporal.
- [x] 3.3 Añadir pruebas de apagado durante rondas activas y ejecutar con detector de carreras.
- [x] 3.4 Actualizar README con semántica de estado, umbrales, URL explícita y efecto de los túneles SSH.
- [x] 3.5 Ejecutar `gofmt`, `go test ./...`, `go test -race ./...` y `go vet ./...`.
