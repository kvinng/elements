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
