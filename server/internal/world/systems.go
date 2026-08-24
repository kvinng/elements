package world

import "math"

const MoveSpeed float32 = 200 // units per second

// systemMovement applies player inputs to entity positions each tick.
// Diagonal movement is clamped to the same speed as cardinal movement.
func systemMovement(w *World, dt float32) {
	for id, input := range w.inputs {
		pos, ok := w.positions[id]
		if !ok {
			continue
		}
		dx, dy := input.MoveX, input.MoveY
		if mag := magnitude(dx, dy); mag > 1.0 {
			dx /= mag
			dy /= mag
		}
		pos.X += dx * MoveSpeed * dt
		pos.Y += dy * MoveSpeed * dt
		w.positions[id] = pos
	}
}

func magnitude(x, y float32) float32 {
	return float32(math.Sqrt(float64(x*x + y*y)))
}
