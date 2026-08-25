package world

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"github.com/kving/games/elements/server/internal/dungeon"
	"github.com/kving/games/elements/server/internal/entity"
)

const (
	TickRate     = 20                     // Hz
	TickDuration = time.Second / TickRate // 50 ms
)

// EntityKind distinguishes entity types in snapshots.
type EntityKind uint8

const (
	KindPlayer     EntityKind = 0
	KindProjectile EntityKind = 1
	KindMob        EntityKind = 2
	KindItem       EntityKind = 3
	KindGoldItem   EntityKind = 4
)

// InputEvent pairs an incoming input with the entity that sent it.
type InputEvent struct {
	Owner entity.EntityID
	State entity.InputState
}

// SpawnRequest creates an entity from outside the simulation loop.
// Result receives the new entity ID after the next tick processes the request.
type SpawnRequest struct {
	Pos   entity.Position
	HP    int32
	El    entity.Element
	Name  string
	Level uint32 // initial level; 1 if zero
	XP    uint32 // initial XP toward next level
	Gold  uint32 // gold accumulated by the player
	Result chan<- entity.EntityID
}

// EntitySnapshot is the portion of entity state broadcast each tick.
type EntitySnapshot struct {
	ID      entity.EntityID    `json:"id"`
	X       float32            `json:"x"`
	Y       float32            `json:"y"`
	HP      int32              `json:"hp"`
	MaxHP   int32              `json:"max_hp"`
	Name    string             `json:"name,omitempty"`
	Kind    EntityKind         `json:"kind"`
	Element entity.ElementType `json:"element"`
	Level   uint32             `json:"level"`
	XP      uint32             `json:"xp,omitempty"`
	XPNext  uint32             `json:"xp_next,omitempty"`
	Gold    uint32             `json:"gold,omitempty"`
}

// WorldSnapshot is the server's authoritative world state at a given tick.
type WorldSnapshot struct {
	Tick     uint64           `json:"tick"`
	Entities []EntitySnapshot `json:"entities"`
}

// World holds all component tables and drives the simulation loop.
//
// Only the goroutine running Run() may read or write component tables.
// All external communication goes through the exported channels.
type World struct {
	nextID      entity.EntityID
	positions   map[entity.EntityID]entity.Position
	healths     map[entity.EntityID]entity.Health
	elements    map[entity.EntityID]entity.Element
	inputs      map[entity.EntityID]entity.InputState
	names       map[entity.EntityID]string
	projectiles map[entity.EntityID]entity.Projectile
	cooldowns   map[entity.EntityID]int32
	ais         map[entity.EntityID]entity.AI
	items       map[entity.EntityID]entity.Item
	levels      map[entity.EntityID]entity.Level
	golds       map[entity.EntityID]uint32

	tick   uint64
	rng    *rand.Rand
	Dungeon *dungeon.Dungeon // nil for open-world zones; read-only after New*

	InputCh    chan InputEvent
	SnapshotCh chan WorldSnapshot
	SpawnCh    chan SpawnRequest
	DespawnCh  chan entity.EntityID
}

func newWorld(rng *rand.Rand) *World {
	return &World{
		nextID:      1,
		positions:   make(map[entity.EntityID]entity.Position),
		healths:     make(map[entity.EntityID]entity.Health),
		elements:    make(map[entity.EntityID]entity.Element),
		inputs:      make(map[entity.EntityID]entity.InputState),
		names:       make(map[entity.EntityID]string),
		projectiles: make(map[entity.EntityID]entity.Projectile),
		cooldowns:   make(map[entity.EntityID]int32),
		ais:         make(map[entity.EntityID]entity.AI),
		items:       make(map[entity.EntityID]entity.Item),
		levels:      make(map[entity.EntityID]entity.Level),
		golds:       make(map[entity.EntityID]uint32),
		rng:         rng,

		InputCh:    make(chan InputEvent, 256),
		SnapshotCh: make(chan WorldSnapshot, 8),
		SpawnCh:    make(chan SpawnRequest, 16),
		DespawnCh:  make(chan entity.EntityID, 16),
	}
}

// New creates an open-world zone (no dungeon tiles, no mobs).
func New() *World { return newWorld(rand.New(rand.NewSource(0))) }

var dungeonTilesets = []string{"walls_floor", "lava", "earth"}

// NewDungeon generates a dungeon zone: BSP tilemap + pre-spawned mobs.
// Tileset is chosen deterministically from the seed so the same seed always produces the same biome.
func NewDungeon(seed int64) *World {
	tileset := dungeonTilesets[seed%int64(len(dungeonTilesets))]
	return NewDungeonWithTheme(seed, tileset)
}

func NewDungeonWithTheme(seed int64, tileset string) *World {
	rng := rand.New(rand.NewSource(seed))
	w := newWorld(rng)
	w.Dungeon = dungeon.Generate(80, 60, rng, seed, tileset)

	// Pre-spawn mobs — safe because Run() hasn't started yet.
	for _, ms := range w.Dungeon.MobSpawns {
		id := w.nextID
		w.nextID++
		lv := ms.Level
		if lv < 1 {
			lv = 1
		}
		maxHP := MobMaxHP(lv)
		w.positions[id] = entity.Position{X: ms.X, Y: ms.Y}
		w.healths[id] = entity.Health{Current: maxHP, Max: maxHP}
		el := entity.ElementType(ms.Element)
		w.elements[id] = entity.Element{Kind: el}
		w.names[id] = "Mob"
		w.ais[id] = entity.AI{State: entity.AIIdle}
		w.levels[id] = entity.Level{Current: lv}
	}
	return w
}

// Run starts the deterministic 20 Hz simulation loop, blocking until ctx is cancelled.
func (w *World) Run(ctx context.Context) {
	ticker := time.NewTicker(TickDuration)
	defer ticker.Stop()

	slog.Info("world loop started", "tick_rate_hz", TickRate, "tick_ms", TickDuration.Milliseconds())

	for {
		select {
		case <-ctx.Done():
			slog.Info("world loop stopped", "last_tick", w.tick)
			return
		case t := <-ticker.C:
			w.tick++
			w.step(float32(TickDuration.Seconds()))
			if elapsed := time.Since(t); elapsed > TickDuration {
				slog.Warn("tick overrun",
					"tick", w.tick,
					"elapsed_ms", elapsed.Milliseconds(),
					"budget_ms", TickDuration.Milliseconds(),
				)
			}
		}
	}
}

func (w *World) step(dt float32) {
	w.processSpawns()
	w.processDespawns()
	w.drainInputs()
	systemCooldowns(w)
	systemShoot(w)
	systemMovement(w, dt)
	systemProjectile(w, dt)
	systemCollision(w)
	systemRespawn(w)
	systemAI(w, dt, w.rng)
	systemItems(w)
	w.emitSnapshot()
}

func (w *World) processSpawns() {
	for {
		select {
		case req := <-w.SpawnCh:
			id := w.nextID
			w.nextID++
			w.positions[id] = req.Pos
			w.healths[id] = entity.Health{Current: req.HP, Max: req.HP}
			w.elements[id] = req.El
			w.names[id] = req.Name
			w.cooldowns[id] = 0
			lv := req.Level
			if lv < 1 {
				lv = 1
			}
			w.levels[id] = entity.Level{Current: lv, XP: req.XP}
			w.golds[id] = req.Gold
			req.Result <- id
		default:
			return
		}
	}
}

func (w *World) processDespawns() {
	for {
		select {
		case id := <-w.DespawnCh:
			delete(w.positions, id)
			delete(w.healths, id)
			delete(w.elements, id)
			delete(w.inputs, id)
			delete(w.names, id)
			delete(w.cooldowns, id)
			delete(w.ais, id)
			delete(w.items, id)
			delete(w.levels, id)
			delete(w.golds, id)
		default:
			return
		}
	}
}

func (w *World) drainInputs() {
	for {
		select {
		case ev := <-w.InputCh:
			w.inputs[ev.Owner] = ev.State
		default:
			return
		}
	}
}

func (w *World) emitSnapshot() {
	snap := WorldSnapshot{
		Tick:     w.tick,
		Entities: make([]EntitySnapshot, 0, len(w.positions)),
	}
	for id, pos := range w.positions {
		h := w.healths[id]
		_, isProj := w.projectiles[id]
		_, isMob := w.ais[id]
		_, isItem := w.items[id]
		var kind EntityKind
		switch {
		case isProj:
			kind = KindProjectile
		case isMob:
			kind = KindMob
		case isItem:
			if w.items[id].Kind == entity.ItemGold {
				kind = KindGoldItem
			} else {
				kind = KindItem
			}
		default:
			kind = KindPlayer
		}
		lv := w.levels[id]
		snap.Entities = append(snap.Entities, EntitySnapshot{
			ID:      id,
			X:       pos.X,
			Y:       pos.Y,
			HP:      h.Current,
			MaxHP:   h.Max,
			Name:    w.names[id],
			Kind:    kind,
			Element: w.elements[id].Kind,
			Level:   lv.Current,
			XP:      lv.XP,
			XPNext:  entity.XPToNext(lv.Current),
			Gold:    w.golds[id],
		})
	}
	select {
	case w.SnapshotCh <- snap:
	default:
	}
}
