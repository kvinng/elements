# Propuesta de Arquitectura de Servidor — Project Elementals

## Opinión sobre las ideas existentes

El stack propuesto (Go + 20 Hz + PostgreSQL/Redis + Godot) es sólido. Go es una elección excelente para esto: las goroutines y los channels encajan de forma natural con el modelo productor/consumidor de un game server, y el garbage collector de Go es lo suficientemente predecible a 50 ms por tick.

Lo que falta en el GDD actual son las piezas que marcan la diferencia entre un servidor que funciona con 10 jugadores y uno que aguanta 500 en el mismo mundo:

- **Área de interés (AOI):** en un MMORPG no puedes mandar el estado del mundo entero a cada cliente cada tick. Sin AOI, escalar es imposible.
- **Predicción en cliente + compensación de lag:** si el servidor es autoritativo al 100% y el cliente espera respuesta antes de moverse, el juego se siente laggy incluso con buena latencia. Es el problema más crítico para la sensación de juego.
- **Modelo de datos orientado a entidades:** sin un esquema claro de cómo viven los datos en memoria durante la simulación, el servidor se vuelve un espagueti de structs acoplados.
- **Protocolo de red:** JSON/WebSockets está bien para prototipar pero tiene demasiado overhead para producción.

---

## Propuesta de Arquitectura

### Visión General

```
  ┌────────────────────────────────────────────────────────────┐
  │                     SERVIDOR (Go)                          │
  │                                                            │
  │  ┌──────────┐   chan Input    ┌──────────────────────┐     │
  │  │ Net Layer│ ──────────────► │   World Loop (20 Hz) │     │
  │  │ (gorout.)│                 │                      │     │
  │  │          │ ◄────────────── │  ECS + Simulación    │     │
  │  └──────────┘   chan Snapshot └──────────┬───────────┘     │
  │                                          │                 │
  │                               ┌──────────▼───────────┐    │
  │                               │  AOI / Spatial Hash  │    │
  │                               └──────────────────────┘    │
  │                                                            │
  │  ┌─────────────┐         ┌────────────────────────────┐    │
  │  │  Auth/API   │         │   Persistence Worker       │    │
  │  │  (HTTP REST)│         │   (async, no bloquea loop) │    │
  │  └─────────────┘         └────────────┬───────────────┘    │
  └────────────────────────────────────────┼───────────────────┘
                                           │
                              ┌────────────▼───────────┐
                              │  PostgreSQL  │  Redis   │
                              └──────────────────────── ┘
```

---

### 1. El Game Loop: un solo goroutine de simulación

El loop de 20 Hz debe correr en **un único goroutine** y ser el único que escribe estado de mundo. Esto elimina locks y condiciones de carrera.

```
cada tick (50 ms):
  1. Drena el canal de inputs (sin bloquear, non-blocking select)
  2. Aplica inputs al estado de entidades
  3. Simula física: movimiento, proyectiles, colisiones
  4. Procesa lógica: daño, cooldowns, muertes, drops
  5. Calcula AOI: qué entidades ve cada jugador
  6. Genera snapshot delta por jugador
  7. Encola snapshots en canal de salida (non-blocking)
```

El goroutine de red (o un pool de ellos) lee del canal de salida y envía a los clientes. Nunca tocan el estado del mundo directamente.

**Por qué 20 Hz y no más:** para un ARPG con proyectiles el límite real no es el CPU del servidor sino el round-trip del cliente. A 20 Hz, el servidor puede interpolar el movimiento en cliente a cualquier FPS. 30 Hz sería mejor si el CPU lo aguanta, pero la arquitectura debe soportarlo fácilmente cambiando el ticker.

---

### 2. ECS mínimo pero explícito

No uses un framework ECS externo; en Go, un ECS casero simple es suficiente y más legible.

```go
// Cada entidad es un uint64 (ID)
type EntityID uint64

// Componentes son structs planos, sin punteros entre ellos
type Position struct{ X, Y float32 }
type Velocity struct{ Dx, Dy float32 }
type Health  struct{ Current, Max int32 }
type Element struct{ Kind ElementType; Level uint8 }
type Projectile struct {
    OwnerID  EntityID
    Damage   int32
    TTL      uint8   // ticks restantes de vida
}

// El mundo es la colección de todas las tablas
type World struct {
    Positions   map[EntityID]Position
    Velocities  map[EntityID]Velocity
    Healths     map[EntityID]Health
    Elements    map[EntityID]Element
    Projectiles map[EntityID]Projectile
    // ...
}
```

Los sistemas son funciones que iteran sobre el world. Sin herencia, sin interfaces pesadas. Esto hace el código de simulación plano y rápido de leer.

---

### 3. AOI con Spatial Hashing

El mundo se divide en celdas de, por ejemplo, 200×200 unidades de juego. Cada entidad vive en una celda. Al generar snapshots, solo se incluyen las entidades en la celda del jugador y las adyacentes (radio configurable).

```
┌───┬───┬───┬───┐
│   │ X │ X │   │   X = celdas visibles para el jugador (●)
├───┼───┼───┼───┤
│ X │ X │●X │ X │
├───┼───┼───┼───┤
│   │ X │ X │   │
└───┴───┴───┴───┘
```

Esto reduce los bytes por tick de O(N entidades totales) a O(entidades locales), que es lo que permite escalar a cientos de jugadores por zona.

---

### 4. Red: WebSockets ahora, UDP después

**Protocolo recomendado:** empezar con WebSockets (gorilla/websocket) porque Godot los soporta de forma nativa y el debugging es inmediato. Migrar a UDP con capa de fiabilidad (ENet o una librería como `pion/dtls`) solo cuando sea necesario.

**Serialización:** Protobuf desde el principio. JSON está bien para debugear pero el overhead en producción es real. Con Protobuf defines los mensajes una vez y generas código para Go y GDScript (o usas el plugin oficial de Godot).

```protobuf
// Mensajes core

message ClientInput {
  uint32 seq     = 1;  // sequence number (para reconciliación)
  float  move_x  = 2;
  float  move_y  = 3;
  uint32 skill_id = 4;
  float  target_x = 5;
  float  target_y = 6;
}

message EntityState {
  uint64 id      = 1;
  float  x       = 2;
  float  y       = 3;
  int32  hp      = 4;
  uint32 anim_id = 5;
}

message WorldSnapshot {
  uint32 tick              = 1;
  uint32 ack_client_seq    = 2;  // último input del cliente procesado
  repeated EntityState entities = 3;
}
```

El campo `ack_client_seq` es clave: permite al cliente descartar predicciones ya confirmadas por el servidor.

---

### 5. Predicción en cliente (Client-Side Prediction)

Este es el punto más impactante para la sensación del juego y el más ignorado en prototipos.

**Sin predicción:** cliente envía input → espera 50 ms a que el servidor responda → aplica movimiento. Resultado: juego laggy incluso en local.

**Con predicción:**
1. El cliente aplica el input **inmediatamente** (predicción local).
2. Envía el input al servidor con un `seq` number.
3. Cuando llega el snapshot del servidor con `ack_client_seq`, el cliente:
   - Descarta los inputs anteriores a `ack_client_seq`.
   - Compara la posición predicha con la del servidor.
   - Si hay diferencia mayor a un umbral, hace un "snap" suave (interpolación).

En Godot esto se implementa manteniendo un buffer circular de inputs y estados predichos. El servidor solo corrige, no dicta frame a frame.

---

### 6. Compensación de lag para proyectiles

Los proyectiles autoritativos (calculados en servidor) deben tener compensación de lag: cuando el servidor procesa un impacto, retrocede el estado del mundo N ticks (donde N = latencia del atacante / 50 ms) antes de calcular el hit. Esto hace que lo que el jugador vio en su pantalla sea lo que "realmente pasó".

Guardar un buffer de snapshots del servidor (últimos 10-20 ticks) en memoria es barato y suficiente para esto.

---

### 7. Persistencia asíncrona, nunca en el loop

**Regla de oro:** el game loop nunca espera a la base de datos.

```
World Loop ──► canal de eventos ──► Persistence Worker goroutine ──► PostgreSQL
```

El worker acumula eventos (movimiento significativo, cambio de HP, item drop, muerte) y los escribe en batch cada N segundos o cuando el buffer se llena. Si el servidor cae, se pierde como máximo el último batch, lo cual es aceptable para un MMORPG.

Redis se usa para:
- Sesiones activas y tokens de autenticación
- Estado de votaciones de guerra (TTL natural en Redis)
- Caché de perfiles leídos frecuentemente

PostgreSQL guarda:
- Personajes, inventario, progresión
- Log de eventos PvP (para auditabilidad del sistema democrático)
- Configuración de facciones y guerras

---

### 8. Estructura de proyecto sugerida

```
avatar-server/
├── cmd/
│   └── server/main.go        # entry point, wiring
├── internal/
│   ├── world/
│   │   ├── world.go          # World struct + tick loop
│   │   ├── systems.go        # movement, combat, projectile systems
│   │   └── aoi.go            # spatial hash
│   ├── entity/
│   │   └── components.go     # todas las structs de componentes
│   ├── net/
│   │   ├── hub.go            # gestiona conexiones WS
│   │   └── codec.go          # serialización protobuf
│   ├── persistence/
│   │   ├── worker.go         # goroutine de escritura async
│   │   └── queries.go        # SQL con pgx
│   └── auth/
│       └── handler.go        # HTTP REST para login/register
├── proto/
│   └── messages.proto
└── go.mod
```

---

### 9. Sobre el copyright de Avatar

El lore de las 4 naciones y los elementos como sistema de magia es lo suficientemente genérico (fuego, agua, tierra, aire existen en infinitos juegos). El riesgo real está en:

- **Nombres propios** de personajes, lugares o organizaciones de la serie.
- **Diseños visuales** específicos (ropa, símbolos, arquitectura).
- **El concepto "Avatar"** como denominación de la clase máxima.

Solución: renombrar la clase Avatar (¿"Primordial"? ya aparece en el GDD), usar nombres de naciones originales, y los sistemas de mecánicas y progresión son seguros. Los elementos como mecánica de juego no son propiedad de Nickelodeon.

---

### Resumen de decisiones clave

| Decisión | Recomendación | Alternativa |
|---|---|---|
| Tick rate | 20 Hz (50 ms) | 30 Hz si CPU lo permite |
| Protocolo | WebSockets (proto) → UDP/ENet | Solo WS si el proyecto no escala |
| Serialización | Protobuf | MessagePack (más simple) |
| AOI | Spatial hashing (celdas fijas) | Quadtree (más complejo, no necesario) |
| Predicción cliente | Sí, desde el inicio | Sin ella el juego se siente mal |
| Persistencia | Async worker con buffer | Nunca síncrona en el loop |
| ECS | Casero, mapas planos | Ningún framework externo |
| Auth | HTTP REST separado del game loop | gRPC si se necesita micro-servicios |
