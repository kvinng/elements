package world

import (
	"context"
	"log/slog"
	"time"

	"github.com/kving/games/elements/server/internal/entity"
)

const (
	TickRate     = 20                     // Hz
	TickDuration = time.Second / TickRate // 50 ms
)

// EntityKind distinguishes players from projectiles in snapshots.
type EntityKind uint8

const (
	KindPlayer     EntityKind = 0
	KindProjectile EntityKind = 1
)

// InputEvent pairs an incoming input with the entity that sent it.
type InputEvent struct {
	Owner entity.EntityID
	State entity.InputState
}

// SpawnRequest creates an entity from outside the simulation loop.
// Result receives the new entity ID after the next tick processes the request.
type SpawnRequest struct {
	Pos    entity.Position
	HP     int32
	El     entity.Element
	Name   string
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
	nextID     entity.EntityID
	positions  map[entity.EntityID]entity.Position
	healths    map[entity.EntityID]entity.Health
	elements   map[entity.EntityID]entity.Element
	inputs     map[entity.EntityID]entity.InputState
	names      map[entity.EntityID]string
	projectiles map[entity.EntityID]entity.Projectile
	cooldowns  map[entity.EntityID]int32

	tick uint64

	InputCh    chan InputEvent
	SnapshotCh chan WorldSnapshot
	SpawnCh    chan SpawnRequest
	DespawnCh  chan entity.EntityID
}

func New() *World {
	return &World{
		nextID:      1,
		positions:   make(map[entity.EntityID]entity.Position),
		healths:     make(map[entity.EntityID]entity.Health),
		elements:    make(map[entity.EntityID]entity.Element),
		inputs:      make(map[entity.EntityID]entity.InputState),
		names:       make(map[entity.EntityID]string),
		projectiles: make(map[entity.EntityID]entity.Projectile),
		cooldowns:   make(map[entity.EntityID]int32),

		InputCh:    make(chan InputEvent, 256),
		SnapshotCh: make(chan WorldSnapshot, 8),
		SpawnCh:    make(chan SpawnRequest, 16),
		DespawnCh:  make(chan entity.EntityID, 16),
	}
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
		kind := KindPlayer
		if _, isProj := w.projectiles[id]; isProj {
			kind = KindProjectile
		}
		snap.Entities = append(snap.Entities, EntitySnapshot{
			ID:      id,
			X:       pos.X,
			Y:       pos.Y,
			HP:      h.Current,
			MaxHP:   h.Max,
			Name:    w.names[id],
			Kind:    kind,
			Element: w.elements[id].Kind,
		})
	}
	select {
	case w.SnapshotCh <- snap:
	default:
	}
}
