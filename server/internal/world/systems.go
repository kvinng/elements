package world

import "math"

// systemMovement applies player inputs to entity positions each tick.
// Movement speed comes from the player's element class stats.
// Diagonal movement is clamped to the same speed as cardinal movement.
// In dungeon zones, positions are pushed out of wall tiles after moving.
func systemMovement(w *World, dt float32) {
	for id, input := range w.inputs {
		pos, ok := w.positions[id]
		if !ok {
			continue
		}
		el := w.elements[id].Kind
		speed := classBaseStats[el].MoveSpeed
		dx, dy := input.MoveX, input.MoveY
		if mag := magnitude(dx, dy); mag > 1.0 {
			dx /= mag
			dy /= mag
		}
		pos.X += dx * speed * dt
		pos.Y += dy * speed * dt
		pos = collideWithTiles(pos, w.Dungeon)
		w.positions[id] = pos
	}
}

func magnitude(x, y float32) float32 {
	return float32(math.Sqrt(float64(x*x + y*y)))
}
