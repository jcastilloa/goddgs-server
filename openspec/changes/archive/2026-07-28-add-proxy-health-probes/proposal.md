## Why

El pool sólo mantiene un booleano transitorio por proxy y los proxies directos no disponen de una comprobación de salida propia. Se requieren sondas activas persistidas para conocer disponibilidad y latencia reales, incluso cuando no hay tráfico de usuarios.

## What Changes

- Añadir sondas HTTP periódicas que salgan a través de cada proxy configurado y midan su latencia.
- Hacer configurables URL, intervalo, timeout y umbrales de éxitos/fallos consecutivos.
- Derivar estados `unknown`, `healthy`, `degraded` y `unhealthy`, manteniendo la señal de conexión SSH como condición adicional.
- Persistir cada resultado de sonda y cada cambio de estado mediante el almacenamiento de operaciones.

## Capabilities

### New Capabilities

- `proxy-health-probing`: Supervisión activa, estado y evolución persistente de la salud de proxies.

### Modified Capabilities

- Ninguna.

## Impact

- Depende de `add-operations-storage`.
- Afecta configuración de proxies, pool de proxy, factory de clientes y ciclo de vida de la aplicación.
- Requiere documentación de configuración y pruebas deterministas del planificador, los umbrales y la cancelación.
