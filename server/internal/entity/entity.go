package entity

type EntityID uint64

type ElementType uint8

const (
	ElementNone  ElementType = iota
	ElementFire
	ElementWater
	ElementEarth
	ElementAir
)

type Position struct{ X, Y float32 }
type Health struct{ Current, Max int32 }

type Element struct {
	Kind  ElementType
	Level uint8
}

// InputState is the most recent input from a client.
// Seq enables client-side prediction reconciliation.
type InputState struct {
	Seq   uint32
	MoveX float32 // normalised [-1, 1]
	MoveY float32 // normalised [-1, 1]
	Fire  bool
	AimX  float32 // normalised aim direction
	AimY  float32
}

// Projectile holds runtime state for a projectile entity.
type Projectile struct {
	OwnerID EntityID
	Damage  int32
	VelX    float32
	VelY    float32
	TTL     int32   // ticks remaining before removal
	Radius  float32 // hit-detection radius (varies by element)
}

// AIState drives mob behaviour.
type AIState uint8

const (
	AIIdle  AIState = iota
	AIChase         // moving toward target player
)

// AI is attached to mob entities.
type AI struct {
	State      AIState
	TargetID   EntityID
	MeleeTimer int32    // ticks until next melee hit is allowed
	LastHitBy  EntityID // last player to deal damage — receives XP on death
}

// Level tracks progression for players and mobs.
type Level struct {
	Current uint32
	XP      uint32 // XP accumulated toward next level (players only)
}

// XPToNext returns XP needed to go from current to current+1.
func XPToNext(current uint32) uint32 { return current * 50 }

// ItemType identifies what a floor item does when picked up.
type ItemType uint8

const (
	ItemHealth ItemType = iota // restores HP
	ItemGold                   // awards gold to the picking player
)

// Item is attached to floor-item entities.
type Item struct {
	Kind   ItemType
	Amount uint32 // gold quantity for ItemGold; unused for ItemHealth
}
