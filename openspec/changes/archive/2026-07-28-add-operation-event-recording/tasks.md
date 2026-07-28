## 1. Fundamentos de registro

- [x] 1.1 Añadir tipos de operación, paso, resultado y categoría de error sobre el puerto de `operations` entregado por `add-operations-storage`.
- [x] 1.2 Implementar saneamiento centralizado de mensajes, URLs y metadatos, con límites de tamaño y eliminación de secretos.
- [x] 1.3 Implementar clasificación estable de cancelación, timeout, rate limit, transporte, 5xx, respuesta inválida, configuración y desconocido.
- [x] 1.4 Implementar el servicio de aplicación para iniciar/finalizar operaciones y pasos usando `context.Context` para correlación.

## 2. Instrumentación de flujos

- [x] 2.1 Añadir middleware selectivo a las rutas de búsqueda, extracción y research para crear y cerrar la operación HTTP raíz.
- [x] 2.2 Decorar el gateway de búsqueda para registrar búsquedas, backend, proxy, duración, resultado y errores.
- [x] 2.3 Decorar los servicios de extracción para registrar extracción heurística y AI sin almacenar contenido de la página.
- [x] 2.4 Decorar clientes LLM de extracción y research para registrar tipo de llamada, modelo, duración y errores saneados.
- [x] 2.5 Propagar el contexto de operación por las fases de research y registrar planificación, búsquedas, extracciones y generación de informe.

## 3. Compatibilidad y verificación

- [x] 3.1 Escribir primero pruebas unitarias de clasificación, saneamiento y límites de texto.
- [x] 3.2 Escribir pruebas de integración con SQLite temporal para operaciones en curso, éxito, error, timeout y cancelación correlacionados.
- [x] 3.3 Añadir pruebas de regresión para que rutas, respuestas, códigos HTTP y bearer de `/v1` no cambien.
- [x] 3.4 Confirmar que ningún registro contiene tokens, claves, cabeceras de autorización, prompts completos, respuestas completas ni cuerpos HTTP.
- [x] 3.5 Ejecutar `gofmt`, `go test ./...`, `go test -race ./...` y `go vet ./...`.
