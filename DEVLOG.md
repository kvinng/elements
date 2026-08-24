# Elements — Devlog

MMORPG/ARPG 2D inspirado en Avatar: The Last Airbender.
Servidor autoritativo en Go, cliente en Godot 4.7.

---

## Arquitectura general

```
client/   → Godot 4.7 (Windows: D:\kevin\dev\elements-client)
server/   → Go, WebSocket autoritativo a 20 Hz
```

El cliente y el servidor se comunican con JSON por WebSocket (`ws://localhost:8080/ws`).
El servidor es la única fuente de verdad: posiciones, daño, colisiones, IA — todo ocurre en Go.
El cliente solo envía input y renderiza lo que el servidor manda.

---

## Servidor Go

### Loop de juego

- Tick a **20 Hz** (50 ms por tick), un solo goroutine ECS.
- Cada tick ejecuta en orden: cooldowns → shoot → projectile → AI → items → respawn → snapshot.
- El snapshot se serializa una vez y se fan-out a todos los clientes conectados.

### Entidades (ECS plano)

Cada entidad es un `EntityID uint32`. Los componentes son mapas separados en `World`:

| Mapa | Contenido |
|------|-----------|
| `positions` | `entity.Position{X, Y float32}` |
| `healths` | `entity.Health{Current, Max int32}` |
| `elements` | `entity.Element{Kind, Level}` |
| `inputs` | `entity.Input{MoveX/Y, Fire, AimX/Y}` — solo jugadores |
| `projectiles` | velocidad, daño, TTL, radio, ownerID |
| `ais` | estado (Idle/Chase), targetID, timer de melee |
| `items` | tipo (Health) |
| `cooldowns` | ticks restantes para volver a disparar |
| `names` | nombre del jugador |

El kind real de una entidad se deduce en snapshot mirando en qué mapas está: player tiene `inputs`, mob tiene `ais`, etc.

### Tipos de entidad (`entity.go`)

```go
type ElementType uint8
const (ElementNone ElementType = iota; ElementFire; ElementWater; ElementEarth; ElementAir)

type AIState uint8
const (AIIdle AIState = iota; AIChase)
type AI struct { State AIState; TargetID EntityID; MeleeTimer int32 }

type ItemType uint8
const (ItemHealth ItemType = iota)
type Item struct { Kind ItemType }
```

### Sistema de clases y elementos (`combat.go`)

Cada elemento tiene stats pasivos de jugador y stats de proyectil:

| Elemento | HP | Velocidad | Proyectil | Daño | Cooldown | TTL |
|----------|-----|-----------|-----------|------|----------|-----|
| None     | 100 | 200       | 550 u/s   | 18   | 8 ticks  | 45  |
| Fuego    | 100 | 210       | 400 u/s   | 20   | 10 ticks | 40  |
| Agua     | 130 | 170       | 300 u/s   | 15   | 12 ticks | 60  |
| Tierra   | 160 | 140       | 200 u/s   | 35   | 15 ticks | 25  |
| Aire     | 70  | 270       | 600 u/s   | 8    | 6 ticks  | 20  |

Aire dispara **3 proyectiles** en abanico (±15°).

**Ciclo de ventajas elementales:** Fuego > Aire > Tierra > Agua > Fuego  
- Ventaja: ×1.5 de daño  
- Desventaja: ×0.7 de daño  
- Sin relación: ×1.0

```go
var elementAdvantage = [5][5]bool{
    /* Fire  */ {false, false, false, false, true},  // Fire beats Air
    /* Water */ {false, true, false, false, false},  // Water beats Fire
    /* Earth */ {false, false, true, false, false},  // Earth beats Water
    /* Air   */ {false, false, false, true, false},  // Air beats Earth
}
```

### Colisión de proyectiles — Swept Sphere (`combat.go`)

Aire lanza proyectiles a 600 u/s = 30 unidades/tick, más grande que el radio de detección (≈18 u).
Sin swept sphere los proyectiles rápidos _tunelean_ a través de los jugadores.

Solución: en vez de testear solo la posición final, se testa el punto más cercano del **segmento** `prevPos → pos` al jugador:

```go
func closestOnSegment(ax, ay, bx, by, px, py float32) (float32, float32) {
    // proyecta (px,py) sobre el segmento, clampea t ∈ [0,1]
}
// hit si dist(closestPoint, player) < PlayerRadius + proj.Radius
```

### Colisión con tiles (`collision.go`, `ai.go`)

Función `collideWithTiles(pos, dungeon)` — circle-vs-AABB push-out:
1. Convierte posición a tile-space.
2. Itera los 3×3 tiles vecinos.
3. Para cada tile Wall: calcula el overlap entre el círculo del jugador y el AABB del tile y empuja fuera.

Usada tanto para jugadores (`systems.go`) como para mobs (`ai.go`).

Para evitar no-determinismo, las IDs se ordenan antes del loop de colisión jugador-vs-jugador:

```go
sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
```

### IA de mobs (`ai.go`)

```
Idle ──(player ≤ 200u)──→ Chase ──(player ≥ 280u)──→ Idle
              Chase: sigue al jugador a 85 u/s + tile collision
              Melee: cada 25 ticks si dist ≤ 22u → 10 dmg al jugador
```

Al morir: se borran de todos los mapas, 40% de probabilidad de dropear un `ItemHealth`.

### Items (`items.go`)

`systemItems` itera ítems cada tick. Si un jugador está a ≤ 20 u del ítem, lo recoge (+40 HP, clampeado a max_hp) y el ítem se elimina.

### Dungeon procedural BSP (`dungeon/gen.go`)

**Binary Space Partitioning:**

1. Rectángulo raíz (80×60 tiles por defecto).
2. `splitAll` divide recursivamente por el eje más largo hasta que los trozos son demasiado pequeños (min = 9 tiles).
3. En cada hoja se coloca una habitación aleatoria dentro del trozo.
4. `connectRooms` sube por el árbol uniendo habitaciones hermanas con corredores en L.
5. `addWalls` rodea cada tile Floor con Wall en los 8 vecinos que sean Void.
6. La habitación 0 es el spawn del jugador. El resto tiene 1–3 mobs.

Con seed=42 y 80×60 tiles se generan ~36 habitaciones y ~66 mobs.

El mapa se codifica como `[]byte` plano (un byte por tile) → JSON lo serializa como **base64** automáticamente por ser `[]byte` en Go. Se pre-encoda una vez al arrancar y se envía a cada cliente al conectar.

### Protocolo WebSocket

**Cliente → Servidor:**
```json
{"type":"input","seq":1,"move_x":-1,"move_y":0,"fire":true,"aim_x":0.7,"aim_y":0.7}
{"type":"chat","text":"hola"}
```

**Servidor → Cliente** (en orden al conectar):
```json
{"type":"welcome","entity_id":1,"name":"Kevin"}
{"type":"map","map_width":80,"map_height":60,"tile_size":32,"tiles":"<base64>","spawn_x":304,"spawn_y":304}
{"type":"snapshot","tick":42,"entities":[...]}
{"type":"chat","entity_id":1,"name":"Kevin","text":"hola"}
```

**Snapshot entity:**
```json
{"id":1,"kind":0,"x":304.0,"y":304.0,"element":1,"hp":100,"max_hp":100,"name":"Kevin"}
```
Kinds: 0=Player, 1=Projectile, 2=Mob, 3=Item

---

## Cliente Godot 4.7

### Estructura de archivos

```
client/
├── project.godot          # autoload Network, main scene Lobby
├── scripts/
│   ├── Network.gd         # autoload singleton — WebSocketPeer
│   ├── Lobby.gd           # selección de nombre y elemento
│   └── Game.gd            # loop de juego, render, input, chat
└── scenes/
    ├── Lobby.tscn
    └── Game.tscn
```

La copia que abre Godot en Windows vive en `D:\kevin\dev\elements-client`.
Los scripts originales están en `client/` dentro del repo WSL y se sincronizan manualmente con `cp -r`.

### Network.gd

Autoload singleton. Usa `WebSocketPeer` (API raw, no multiplayer API) para control total del protocolo JSON.

```gdscript
const SERVER_URL := "ws://localhost:8080/ws"
# Señales emitidas al resto de escenas:
signal connected(entity_id, name)
signal snapshot_received(entities)
signal map_received(data)
signal chat_received(entity_id, name, text)
signal disconnected()
```

`_process` llama a `_ws.poll()` cada frame y despacha paquetes.

### Lobby.gd

Muestra botones de elemento con stats (HP, velocidad, ventaja). Al pulsar "Entrar" llama a `Network.connect_to_server(nombre, elemento)` y cambia a `Game.tscn`.

### Game.gd — render procedural

**Dungeon:** Al recibir `map_received`, decodifica el base64 con `Marshalls.base64_to_raw()`, dibuja cada tile en un `Image` (RGB8) y lo convierte a `ImageTexture` → `Sprite2D`. Se hace una sola vez.

**Entidades:** Por cada entidad en el snapshot se crea/actualiza un `Node2D`. El draw es procedural (señal `draw`):

| Kind | Visual |
|------|--------|
| 0 Player | Círculo con color elemental, borde blanco si es el propio, HP bar, nombre |
| 1 Projectile | Punto con glow, tamaño según elemento (Tierra = más grande) |
| 2 Mob | Diamante (polígono 4 vértices), HP bar |
| 3 Item | Orbe verde animado (bob con `sin(time)`) |

**Input:** A 20 Hz (igual que el servidor), lee WASD + mouse + Fire y llama `Network.send_input(...)`.

**Cámara:** `Camera2D` sigue al jugador propio; zoom = 2×.

**Chat:** `RichTextLabel` con BBCode + `LineEdit`. Enter activa/envía.

---

## Setup de desarrollo

### Arrancar el servidor

```bash
cd server
go run ./cmd/server/ > /tmp/server.log 2>&1 &
```

Puerto: `8080`. Dungon seed fijo: `42`.

Matar si hay conflicto de puerto:
```bash
ss -tlnp | grep 8080 | awk '{print $NF}' | grep -oP 'pid=\K[0-9]+' | xargs kill -9
```

### Abrir el cliente Godot desde WSL

**Ruta del ejecutable:** `D:\kevin\Desktop\Godot_v4.7-stable_win64.exe`  
**Proyecto en Windows filesystem:** `D:\kevin\dev\elements-client\`

El proyecto **debe vivir en el filesystem de Windows** (`D:\`), no en WSL (`/home/...`).
La ruta UNC `\\wsl.localhost\Ubuntu\home\...` no funciona en este entorno.

**Comando para abrir desde WSL:**
```bash
cmd.exe /c start "" "D:\kevin\Desktop\Godot_v4.7-stable_win64.exe" "D:\kevin\dev\elements-client\project.godot"
```

`cmd.exe` está disponible en WSL2 como interop de Windows. El flag `/c start ""` lanza el proceso de forma desacoplada (no bloquea el terminal).

**Por qué no funciona lanzarlo directamente con el path `/mnt/d/...`:**
```bash
# Esto falla con exit code 144 (archivo no ejecutable desde WSL directamente):
"/mnt/d/kevin/Desktop/Godot_v4.7-stable_win64.exe" "D:\\kevin\\dev\\elements-client\\project.godot"
```
Los ejecutables `.exe` de Windows necesitan el interop de `cmd.exe` o `powershell.exe` para lanzarse correctamente desde WSL cuando se pasan argumentos con rutas Windows.

**Verificar que abrió:**
```bash
tasklist.exe | grep -i godot
# Godot_v4.7-stable_win64.e  34416 Console  1  1.325.280 KB
```

### Sincronizar cambios del cliente WSL → Windows

```bash
cp -r /home/kevin/src/github/kving/games/elements/client/* /mnt/d/kevin/dev/elements-client/
```

### Cliente HTML5 de debug

`server/cmd/server/static/index.html` — se sirve en `http://localhost:8080/`. Tiene dungeon rendering, mobs, items y stats elementales. Útil para probar sin abrir Godot.

---

## Bugs resueltos

| Bug | Causa | Fix |
|-----|-------|-----|
| Proyectiles de Aire tuneleaban | 600 u/s = 30 u/tick > radio detección 18u | Swept sphere: `closestOnSegment` sobre el segmento prevPos→pos |
| Push de colisión no determinista | Iteración de map en Go no tiene orden | `sort.Slice(ids, ...)` antes del loop |
| Kind de entidad incorrecto | `w.projectiles[id] != (entity.Projectile{})` falla con zero-values | Map lookup: `_, isProj := w.projectiles[id]` |
| Conflicto de puerto 8080 | Procesos `go run` anteriores siguen vivos | `ss -tlnp | grep 8080 | ... | xargs kill -9` |
| Godot no abría con ruta UNC | `\\wsl.localhost\` no funciona en este Windows | Copiar proyecto a `D:\kevin\dev\elements-client` |

---

## Roadmap / ideas pendientes

- [ ] Sprites y animaciones (reemplazar draw procedural)
- [ ] Partículas de impacto y muerte
- [ ] Segunda habilidad por elemento (Q o shift)
- [ ] Árbol de habilidades / progresión
- [ ] Jefes de dungeon (boss rooms)
- [ ] Múltiples zonas con portales (overworld + dungeons)
- [ ] Persistencia de jugadores (base de datos)
- [ ] Minimap
- [ ] Exportar a mobile / browser desde Godot
