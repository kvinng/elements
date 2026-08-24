package world

import "github.com/kving/games/elements/server/internal/entity"

// PlayerRadius is the collision circle radius for player entities (units).
const PlayerRadius float32 = 14

// systemCollision resolves circle-circle overlap between player entities.
// Projectiles are excluded; they use hit detection in systemProjectile.
func systemCollision(w *World) {
	ids := make([]entity.EntityID, 0, len(w.inputs))
	for id := range w.inputs {
		ids = append(ids, id)
	}

	const minDist = PlayerRadius * 2

	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a, b := ids[i], ids[j]
			pa, okA := w.positions[a]
			pb, okB := w.positions[b]
			if !okA || !okB {
				continue
			}
			dx := pb.X - pa.X
			dy := pb.Y - pa.Y
			dist := magnitude(dx, dy)
			if dist >= minDist {
				continue
			}
			if dist == 0 {
				// Degenerate: exact same position — push apart on x-axis.
				pa.X -= minDist / 2
				pb.X += minDist / 2
				w.positions[a] = pa
				w.positions[b] = pb
				continue
			}
			overlap := (minDist - dist) / 2
			nx := dx / dist
			ny := dy / dist
			pa.X -= nx * overlap
			pa.Y -= ny * overlap
			pb.X += nx * overlap
			pb.Y += ny * overlap
			w.positions[a] = pa
			w.positions[b] = pb
		}
	}
}
