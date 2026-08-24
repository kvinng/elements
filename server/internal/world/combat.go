package world

import "github.com/kving/games/elements/server/internal/entity"

const (
	RespawnX float32 = 400
	RespawnY float32 = 300
)

// elemStats defines the projectile behaviour for each element.
type elemStats struct {
	Speed    float32
	Damage   int32
	TTL      int32
	Radius   float32
	Cooldown int32
	Spread   bool // fires 3 projectiles in ±15° spread (Air)
}

// classStats defines passive player attributes per element.
type classStats struct {
	BaseHP    int32   // starting max HP
	MoveSpeed float32 // units per second
}

// elementStats is indexed by entity.ElementType (0=None … 4=Air).
var elementStats = [5]elemStats{
	/* None  */ {Speed: 550, Damage: 18, TTL: 45, Radius: 5, Cooldown: 8},
	/* Fire  */ {Speed: 400, Damage: 20, TTL: 40, Radius: 5, Cooldown: 10},
	/* Water */ {Speed: 300, Damage: 15, TTL: 60, Radius: 6, Cooldown: 12},
	/* Earth */ {Speed: 200, Damage: 35, TTL: 25, Radius: 8, Cooldown: 15},
	/* Air   */ {Speed: 600, Damage: 8, TTL: 20, Radius: 4, Cooldown: 6, Spread: true},
}

// classBaseStats indexed by entity.ElementType.
//
// Fire  — balanced
// Water — tanky, slow
// Earth — very tanky, very slow
// Air   — fragile, very fast
var classBaseStats = [5]classStats{
	/* None  */ {BaseHP: 100, MoveSpeed: 200},
	/* Fire  */ {BaseHP: 100, MoveSpeed: 210},
	/* Water */ {BaseHP: 130, MoveSpeed: 170},
	/* Earth */ {BaseHP: 160, MoveSpeed: 140},
	/* Air   */ {BaseHP: 70, MoveSpeed: 270},
}

// elementAdvantage[attacker][defender] → true when attacker beats defender.
// Cycle: Fire > Air > Earth > Water > Fire.
var elementAdvantage = [5][5]bool{
	/* None  */ {},
	/* Fire  */ {false, false, false, false, true},  // Fire beats Air
	/* Water */ {false, true, false, false, false},  // Water beats Fire
	/* Earth */ {false, false, true, false, false},  // Earth beats Water
	/* Air   */ {false, false, false, true, false},  // Air beats Earth
}

// damageMultiplier returns 1.5 if attacker beats defender, 0.7 if defender beats attacker.
func damageMultiplier(attacker, defender entity.ElementType) float32 {
	if attacker == entity.ElementNone || defender == entity.ElementNone {
		return 1.0
	}
	if elementAdvantage[attacker][defender] {
		return 1.5
	}
	if elementAdvantage[defender][attacker] {
		return 0.7
	}
	return 1.0
}

// sin/cos of 15° for the Air spread.
const (
	spread15sin float32 = 0.25882
	spread15cos float32 = 0.96593
)

// systemCooldowns decrements all per-entity shoot cooldowns each tick.
func systemCooldowns(w *World) {
	for id, cd := range w.cooldowns {
		if cd > 0 {
			w.cooldowns[id] = cd - 1
		}
	}
}

// systemShoot reads Fire inputs and spawns projectile entities using per-element stats.
func systemShoot(w *World) {
	for id, input := range w.inputs {
		if !input.Fire || w.cooldowns[id] > 0 {
			continue
		}
		pos, ok := w.positions[id]
		if !ok {
			continue
		}

		el := w.elements[id]
		s := elementStats[el.Kind]

		ax, ay := input.AimX, input.AimY
		if mag := magnitude(ax, ay); mag > 0 {
			ax /= mag
			ay /= mag
		} else {
			ax = 1
		}

		// Directions to shoot: one or three for Air spread.
		dirs := [3][2]float32{{ax, ay}}
		count := 1
		if s.Spread {
			lx, ly := rotDir(ax, ay, -spread15sin, spread15cos)
			rx, ry := rotDir(ax, ay, spread15sin, spread15cos)
			dirs[0] = [2]float32{lx, ly}
			dirs[1] = [2]float32{ax, ay}
			dirs[2] = [2]float32{rx, ry}
			count = 3
		}

		offset := PlayerRadius + s.Radius + 2
		for i := 0; i < count; i++ {
			dx, dy := dirs[i][0], dirs[i][1]
			projID := w.nextID
			w.nextID++
			w.positions[projID] = entity.Position{X: pos.X + dx*offset, Y: pos.Y + dy*offset}
			w.healths[projID] = entity.Health{Current: 1, Max: 1}
			w.elements[projID] = el // projectile inherits shooter's element for snapshot colour
			w.projectiles[projID] = entity.Projectile{
				OwnerID: id,
				Damage:  s.Damage,
				VelX:    dx * s.Speed,
				VelY:    dy * s.Speed,
				TTL:     s.TTL,
				Radius:  s.Radius,
			}
		}
		w.cooldowns[id] = s.Cooldown
	}
}

// systemProjectile moves projectiles and checks for player hits.
func systemProjectile(w *World, dt float32) {
	var toRemove []entity.EntityID

	for id, proj := range w.projectiles {
		prevPos := w.positions[id]
		pos := prevPos
		pos.X += proj.VelX * dt
		pos.Y += proj.VelY * dt
		w.positions[id] = pos

		proj.TTL--
		hit := false

		if proj.TTL > 0 {
			for playerID := range w.inputs {
				if playerID == proj.OwnerID {
					continue
				}
				ppos, ok := w.positions[playerID]
				if !ok {
					continue
				}
				// Swept sphere: test closest point on (prevPos → pos) to the player.
				// This prevents fast projectiles from tunnelling through players.
				cx, cy := closestOnSegment(prevPos.X, prevPos.Y, pos.X, pos.Y, ppos.X, ppos.Y)
				if magnitude(ppos.X-cx, ppos.Y-cy) < PlayerRadius+proj.Radius {
					projEl := w.elements[id].Kind
					targetEl := w.elements[playerID].Kind
					mult := damageMultiplier(projEl, targetEl)
					dmg := int32(float32(proj.Damage) * mult)
					h := w.healths[playerID]
					h.Current -= dmg
					if h.Current < 0 {
						h.Current = 0
					}
					w.healths[playerID] = h
					hit = true
					break
				}
			}
		}

		if proj.TTL <= 0 || hit {
			toRemove = append(toRemove, id)
		} else {
			w.projectiles[id] = proj
		}
	}

	for _, id := range toRemove {
		delete(w.positions, id)
		delete(w.projectiles, id)
		delete(w.healths, id)
		delete(w.elements, id)
	}
}

// systemRespawn teleports any player whose HP hit zero back to spawn.
func systemRespawn(w *World) {
	for id, h := range w.healths {
		if _, isProj := w.projectiles[id]; isProj {
			continue
		}
		if h.Current <= 0 {
			h.Current = h.Max
			w.healths[id] = h
			w.positions[id] = entity.Position{X: RespawnX, Y: RespawnY}
		}
	}
}

// BaseHP returns the starting max-health for an element class.
func BaseHP(el entity.ElementType) int32 {
	return classBaseStats[el].BaseHP
}

// rotDir rotates (x,y) by angle defined by (s=sin, c=cos).
func rotDir(x, y, s, c float32) (float32, float32) {
	return x*c - y*s, x*s + y*c
}

// closestOnSegment returns the closest point on segment (ax,ay)→(bx,by) to (px,py).
func closestOnSegment(ax, ay, bx, by, px, py float32) (float32, float32) {
	dx, dy := bx-ax, by-ay
	lenSq := dx*dx + dy*dy
	if lenSq == 0 {
		return ax, ay
	}
	t := ((px-ax)*dx + (py-ay)*dy) / lenSq
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return ax + t*dx, ay + t*dy
}
