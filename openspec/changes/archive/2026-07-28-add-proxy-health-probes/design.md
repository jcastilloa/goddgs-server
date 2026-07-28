## Context

El pool selecciona clientes por un booleano de salud. Los túneles SSH informan conexión, pero los proxies directos comienzan sanos y no se comprueba su salida. Las sondas necesitan reutilizar la configuración de cada proxy y el almacenamiento SQLite de operaciones.

## Goals / Non-Goals

**Goals:**

- Comprobar periódicamente una URL HTTP configurada a través de cada proxy.
- Medir latencia y persistir cada resultado y transición de estado.
- Evitar oscilaciones con umbrales de éxitos y fallos consecutivos.
- Integrar conexión SSH y salud de sonda con el pool de selección.

**Non-Goals:**

- Descubrir proxies, cambiar su configuración en caliente o verificar anonimato/geolocalización.
- Usar una URL de terceros implícita o sondear rutas de búsqueda reales.
- Crear una interfaz de visualización.

## Decisions

### Configuración global de sondas y URL explícita

Se añadirá `operations.probe` con `url`, `interval`, `timeout`, `success_threshold` y `failure_threshold`. Si las sondas están habilitadas, la URL será obligatoria, HTTP(S) y establecida por el operador. La validación ocurrirá al arrancar; no se elige un proveedor externo por defecto.

### Cliente por proxy y solicitud mínima

El factory conservará o expondrá la URL efectiva por proxy (incluido el SOCKS local de SSH) para construir un `http.Client` de sonda aislado. La sonda hará una petición `GET` con contexto de timeout y considerará éxito sólo una respuesta 2xx/3xx. Cualquier error de transporte, timeout o 4xx/5xx será fallo y se cerrará el body en todos los casos.

### Máquina de estados con histéresis

Los estados iniciales serán `unknown`. Un proxy será `healthy` después del umbral de éxitos, `degraded` cuando tenga fallos pero no alcance el umbral de caída y `unhealthy` al alcanzar el umbral de fallos. Una transición se persiste una sola vez por cambio. Un túnel SSH desconectado fuerza `unhealthy`; reconectarse vuelve a `unknown` hasta una sonda exitosa, evitando marcar saludable sólo por tener sesión SSH.

### Supervisor cancelable y sin bloqueos globales

Un supervisor se inicia después del gateway y usa el contexto principal para lanzar rondas. Cada ronda tiene una sonda por proxy, con concurrencia limitada al número de proxies; el apagado cancela las peticiones y espera las goroutines. La actualización del pool reutiliza su mutex y no retiene el bloqueo durante I/O o escrituras SQLite.

### Persistencia independiente de tráfico real

Cada sonda crea una fila con hora, duración, resultado HTTP si lo hay, clasificación de error y estado resultante. Las transiciones se almacenan separadamente para que el panel pueda calcular disponibilidad y periodos caídos. El tráfico real seguirá aportando datos a través del cambio de eventos, pero no sustituye las sondas activas.

## Risks / Trade-offs

- [La URL de sonda bloquea o factura tráfico] → el operador la configura y define timeout; una solicitud mínima y el intervalo acotan el impacto.
- [Falsos positivos por fallos transitorios] → umbrales configurables e histéresis con estado `degraded`.
- [Un proxy SSH se reconecta repetidamente] → la señal de túnel fuerza estado no saludable y todas las rondas respetan el contexto de cancelación.
- [El endpoint devuelve redirecciones] → 3xx cuenta como salida HTTP satisfactoria, sin seguir redirecciones fuera del proxy.

## Migration Plan

1. Añadir configuración, validación y documentación de `operations.probe`.
2. Exponer los datos de transporte por proxy que necesita la sonda sin publicar secretos.
3. Implementar máquina de estados, repositorio de resultados y supervisor cancelable.
4. Integrar inicio y cierre junto al gateway; probar con transportes HTTP controlados y reloj/planificador determinista.

Para revertir, detener el supervisor; los resultados existentes siguen sujetos a la retención de SQLite.

## Open Questions

- Ninguna: las sondas se habilitan explícitamente mediante la configuración para no generar tráfico inesperado.
