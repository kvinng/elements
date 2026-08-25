package world

import (
	"log/slog"

	"github.com/kving/games/elements/server/internal/dungeon"
	"github.com/kving/games/elements/server/internal/entity"
)

const (
	aggroRange       float32 = 200 // units: enter chase when player is closer than this
	deaggroRange     float32 = 280 // units: give up chase when player is farther than this
	meleeRange       float32 = 22  // units: deal melee damage when this close
	meleeCooldown    int32   = 25  // ticks between hits (~1.25 s)
	mobSpeed         float32 = 85  // units/sec
	itemDropChance           = 40  // percent
	maxLevel         uint32  = 100
	goldProtectTicks int32   = 200 // 10 s @ 20 Hz — only the killer can pick up gold during this window
)

// systemAI drives mob behaviour and handles mob death + item drops.
func systemAI(w *World, dt float32, rng interface{ Intn(int) int }) {
	var dead []entity.EntityID

	for id, ai := range w.ais {
		if ai.MeleeTimer > 0 {
			ai.MeleeTimer--
		}
		h := w.healths[id]
		if h.Current <= 0 {
			dead = append(dead, id)
			continue
		}

		pos := w.positions[id]

		// ── target selection ──────────────────────────────────────────────────
		var nearestDist float32 = deaggroRange + 1
		var nearestID entity.EntityID
		for pid := range w.inputs {
			ppos := w.positions[pid]
			d := magnitude(ppos.X-pos.X, ppos.Y-pos.Y)
			if d < nearestDist {
				nearestDist = d
				nearestID = pid
			}
		}

		switch ai.State {
		case entity.AIIdle:
			if nearestDist <= aggroRange {
				ai.State = entity.AIChase
				ai.TargetID = nearestID
			}
		case entity.AIChase:
			// Check if current target is still valid and close enough.
			if ai.TargetID == 0 || nearestDist > deaggroRange {
				ai.State = entity.AIIdle
				ai.TargetID = 0
				break
			}
			// Re-lock on closest player.
			ai.TargetID = nearestID

			tpos := w.positions[ai.TargetID]
			dx := tpos.X - pos.X
			dy := tpos.Y - pos.Y
			dist := magnitude(dx, dy)

			if dist <= meleeRange {
				// Melee attack — damage scales with mob level.
				if ai.MeleeTimer <= 0 {
					mobLv := w.levels[id].Current
					th := w.healths[ai.TargetID]
					th.Current -= MobMeleeDmg(mobLv)
					if th.Current < 0 {
						th.Current = 0
					}
					w.healths[ai.TargetID] = th
					ai.MeleeTimer = meleeCooldown
				}
			} else {
				// Move toward target.
				nx, ny := dx/dist, dy/dist
				pos.X += nx * mobSpeed * dt
				pos.Y += ny * mobSpeed * dt
				// Tile collision.
				pos = collideWithTiles(pos, w.Dungeon)
				w.positions[id] = pos
			}
		}
		w.ais[id] = ai
	}

	// ── mob death: award XP, drop item, remove entity ────────────────────────
	for _, id := range dead {
		ai := w.ais[id]
		mobLv := w.levels[id].Current
		pos := w.positions[id]

		// Award XP to the player who landed the killing blow.
		if ai.LastHitBy != 0 {
			if lv, ok := w.levels[ai.LastHitBy]; ok {
				xpGain := MobXPReward(mobLv)
				lv.XP += xpGain
				// Level-up loop (can gain multiple levels from one kill at low levels).
				for lv.Current < maxLevel {
					need := entity.XPToNext(lv.Current)
					if lv.XP < need {
						break
					}
					lv.XP -= need
					lv.Current++
					// Increase max HP and heal by the HP gain.
					el := w.elements[ai.LastHitBy].Kind
					newMax := BaseHP(el, lv.Current)
					h := w.healths[ai.LastHitBy]
					h.Max = newMax
					h.Current = h.Max // level-up restaura HP completo
					w.healths[ai.LastHitBy] = h
					slog.Info("level up", "entity", ai.LastHitBy, "level", lv.Current)
				}
				w.levels[ai.LastHitBy] = lv
			}
		}

		delete(w.positions, id)
		delete(w.healths, id)
		delete(w.elements, id)
		delete(w.names, id)
		delete(w.ais, id)
		delete(w.levels, id)

		if rng.Intn(100) < itemDropChance {
			itemID := w.nextID
			w.nextID++
			w.positions[itemID] = pos
			w.items[itemID] = entity.Item{Kind: entity.ItemHealth}
		}
		// Always drop gold (amount: mob_level × rand(1..3)).
		// Protected for goldProtectTicks ticks — only the killer can pick it up during that window.
		goldID := w.nextID
		w.nextID++
		w.positions[goldID] = pos
		w.items[goldID] = entity.Item{
			Kind:         entity.ItemGold,
			Amount:       uint32(mobLv) * uint32(rng.Intn(3)+1),
			Owner:        ai.LastHitBy,
			ProtectTicks: goldProtectTicks,
		}
	}
}

// collideWithTiles pushes pos out of any wall tile the player circle overlaps.
func collideWithTiles(pos entity.Position, dng *dungeon.Dungeon) entity.Position {
	if dng == nil {
		return pos
	}
	const r = PlayerRadius
	const ts = dungeon.TileSize

	x0 := int((pos.X - r) / ts)
	x1 := int((pos.X + r) / ts)
	y0 := int((pos.Y - r) / ts)
	y1 := int((pos.Y + r) / ts)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= dng.Width {
		x1 = dng.Width - 1
	}
	if y1 >= dng.Height {
		y1 = dng.Height - 1
	}

	for ty := y0; ty <= y1; ty++ {
		for tx := x0; tx <= x1; tx++ {
			if dng.Tiles[ty][tx] != dungeon.TileWall {
				continue
			}
			left := float32(tx * ts)
			right := float32((tx + 1) * ts)
			top := float32(ty * ts)
			bot := float32((ty + 1) * ts)

			// Closest point on the tile AABB to the circle centre.
			cx := pos.X
			if cx < left {
				cx = left
			}
			if cx > right {
				cx = right
			}
			cy := pos.Y
			if cy < top {
				cy = top
			}
			if cy > bot {
				cy = bot
			}

			dx := pos.X - cx
			dy := pos.Y - cy
			dist := magnitude(dx, dy)
			if dist == 0 || dist >= r {
				continue
			}
			// Push the circle out.
			nx := dx / dist
			ny := dy / dist
			overlap := r - dist
			pos.X += nx * overlap
			pos.Y += ny * overlap
		}
	}
	return pos
}
