package world

import (
	"testing"

	"github.com/kving/games/elements/server/internal/entity"
)

func TestDungeonGeneration(t *testing.T) {
	w := NewDungeon(42)
	if w.Dungeon == nil {
		t.Fatal("dungeon is nil")
	}
	if len(w.Dungeon.Rooms) == 0 {
		t.Fatal("no rooms generated")
	}
	if w.Dungeon.PlayerSpawn == ([2]float32{}) {
		t.Fatal("player spawn is zero")
	}
	if len(w.ais) == 0 {
		t.Fatal("no mobs pre-spawned")
	}
	t.Logf("rooms=%d  mobs=%d  spawn=(%.0f,%.0f)",
		len(w.Dungeon.Rooms), len(w.ais),
		w.Dungeon.PlayerSpawn[0], w.Dungeon.PlayerSpawn[1])
}

func TestMobAggro(t *testing.T) {
	w := NewDungeon(42)

	// Find any mob.
	var mobID entity.EntityID
	for id := range w.ais {
		mobID = id
		break
	}
	if mobID == 0 {
		t.Skip("no mobs spawned")
	}
	mobPos := w.positions[mobID]

	// Add a fake player immediately next to the mob (within aggro range).
	playerID := w.nextID
	w.nextID++
	w.positions[playerID] = entity.Position{X: mobPos.X + 10, Y: mobPos.Y}
	w.healths[playerID] = entity.Health{Current: 100, Max: 100}
	w.inputs[playerID] = entity.InputState{}

	dt := float32(TickDuration.Seconds())
	for i := 0; i < 5; i++ {
		systemAI(w, dt, w.rng)
	}

	ai := w.ais[mobID]
	if ai.State != entity.AIChase {
		t.Errorf("mob should be chasing after 5 ticks near player, got state=%d", ai.State)
	}
	t.Logf("mob #%d is chasing player #%d", mobID, ai.TargetID)
}

func TestItemDrop(t *testing.T) {
	w := NewDungeon(99) // different seed for variety
	initialItems := len(w.items)

	var mobID entity.EntityID
	for id := range w.ais {
		mobID = id
		break
	}
	if mobID == 0 {
		t.Skip("no mobs spawned")
	}

	// Kill the mob by zeroing its HP.
	h := w.healths[mobID]
	h.Current = 0
	w.healths[mobID] = h

	dt := float32(TickDuration.Seconds())
	// Run systemAI multiple times to ensure drop happens (40% chance × runs).
	for i := 0; i < 20; i++ {
		systemAI(w, dt, w.rng)
	}

	if _, stillAlive := w.ais[mobID]; stillAlive {
		t.Error("mob with 0 HP should have been removed by systemAI")
	}
	t.Logf("items before=%d after=%d", initialItems, len(w.items))
}
