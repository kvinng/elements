// Package dungeon generates tile-based dungeons using Binary Space Partitioning.
//
// BSP explained: take a rectangle, split it in half (H or V), recurse on each
// half until the pieces are small enough. Place a room in every leaf.
// Connect each pair of sibling rooms with an L-shaped corridor.
// Finally, surround floor tiles with wall tiles.
package dungeon

import "math/rand"

// TileSize is the edge length of one tile in world units.
const TileSize = 32

// TileType values stored in Dungeon.Tiles.
type TileType uint8

const (
	TileVoid  TileType = 0
	TileWall  TileType = 1
	TileFloor TileType = 2
)

// Rect is a tile-space rectangle (not world-space).
type Rect struct{ X, Y, W, H int }

func (r Rect) CenterX() int { return r.X + r.W/2 }
func (r Rect) CenterY() int { return r.Y + r.H/2 }

// MobSpawn carries world-space coordinates, element and level for one mob.
type MobSpawn struct {
	X, Y    float32
	Element uint8
	Level   uint32
}

// Dungeon is the fully generated level.
type Dungeon struct {
	Width, Height int          // in tiles
	Tiles         [][]TileType // [y][x]; index as Tiles[ty][tx]
	Rooms         []Rect       // all rooms (index 0 = player spawn room)
	PlayerSpawn   [2]float32   // world-space centre of Rooms[0]
	MobSpawns     []MobSpawn
}

// Generate builds a dungeon of (width × height) tiles using rng for all
// random decisions. width and height should be at least 32.
func Generate(width, height int, rng *rand.Rand) *Dungeon {
	d := &Dungeon{Width: width, Height: height}
	d.Tiles = make([][]TileType, height)
	for y := range d.Tiles {
		d.Tiles[y] = make([]TileType, width)
	}

	root := &bspLeaf{rect: Rect{0, 0, width, height}}
	root.splitAll(rng, 9)
	root.createRooms(rng)
	d.collectRooms(root)

	// Paint rooms as floor tiles.
	for _, r := range d.Rooms {
		d.fillRect(r, TileFloor)
	}

	// Connect sibling rooms with L-shaped corridors.
	root.connectRooms(d)

	// Add wall tiles around any floor tile that borders void.
	d.addWalls()

	// Player spawns in the centre of the first room.
	if len(d.Rooms) > 0 {
		r := d.Rooms[0]
		d.PlayerSpawn = worldCenter(r)
	}

	// Scatter mobs in every room except the first (player spawn).
	// Mob level increases with room depth: rooms farther from spawn → higher level.
	elems := []uint8{1, 2, 3, 4} // Fire, Water, Earth, Air
	total := len(d.Rooms)
	for i, r := range d.Rooms {
		if i == 0 {
			continue
		}
		count := 1 + rng.Intn(3)
		for j := 0; j < count; j++ {
			tx := r.X + 1 + rng.Intn(max(1, r.W-2))
			ty := r.Y + 1 + rng.Intn(max(1, r.H-2))
			d.MobSpawns = append(d.MobSpawns, MobSpawn{
				X:       float32(tx*TileSize + TileSize/2),
				Y:       float32(ty*TileSize + TileSize/2),
				Element: elems[rng.Intn(len(elems))],
				Level:   mobLevel(i, total, rng),
			})
		}
	}

	return d
}

// worldCenter returns the world-space (X, Y) of the centre of a tile rect.
func worldCenter(r Rect) [2]float32 {
	return [2]float32{
		float32(r.CenterX()*TileSize + TileSize/2),
		float32(r.CenterY()*TileSize + TileSize/2),
	}
}

func (d *Dungeon) fillRect(r Rect, t TileType) {
	for y := r.Y; y < r.Y+r.H && y < d.Height; y++ {
		for x := r.X; x < r.X+r.W && x < d.Width; x++ {
			if y >= 0 && x >= 0 {
				d.Tiles[y][x] = t
			}
		}
	}
}

func (d *Dungeon) hCorridor(x1, x2, y int) {
	lo, hi := x1, x2
	if lo > hi {
		lo, hi = hi, lo
	}
	for x := lo; x <= hi; x++ {
		if y >= 0 && y < d.Height && x >= 0 && x < d.Width {
			d.Tiles[y][x] = TileFloor
		}
	}
}

func (d *Dungeon) vCorridor(y1, y2, x int) {
	lo, hi := y1, y2
	if lo > hi {
		lo, hi = hi, lo
	}
	for y := lo; y <= hi; y++ {
		if y >= 0 && y < d.Height && x >= 0 && x < d.Width {
			d.Tiles[y][x] = TileFloor
		}
	}
}

func (d *Dungeon) addWalls() {
	for y := 0; y < d.Height; y++ {
		for x := 0; x < d.Width; x++ {
			if d.Tiles[y][x] != TileFloor {
				continue
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					ny, nx := y+dy, x+dx
					if ny >= 0 && ny < d.Height && nx >= 0 && nx < d.Width {
						if d.Tiles[ny][nx] == TileVoid {
							d.Tiles[ny][nx] = TileWall
						}
					}
				}
			}
		}
	}
}

func (d *Dungeon) collectRooms(l *bspLeaf) {
	if l == nil {
		return
	}
	if l.room != nil {
		d.Rooms = append(d.Rooms, *l.room)
	}
	d.collectRooms(l.left)
	d.collectRooms(l.right)
}

// ── BSP leaf ──────────────────────────────────────────────────────────────────

type bspLeaf struct {
	rect        Rect
	left, right *bspLeaf
	room        *Rect
}

func (l *bspLeaf) splitAll(rng *rand.Rand, minSize int) {
	if l.left != nil || l.right != nil {
		return
	}
	if !l.split(rng, minSize) {
		return
	}
	l.left.splitAll(rng, minSize)
	l.right.splitAll(rng, minSize)
}

func (l *bspLeaf) split(rng *rand.Rand, minSize int) bool {
	splitH := rng.Intn(2) == 0
	// Prefer splitting the longer axis when there's a large aspect ratio.
	if l.rect.W > l.rect.H && float64(l.rect.W)/float64(l.rect.H) >= 1.25 {
		splitH = false
	} else if l.rect.H > l.rect.W && float64(l.rect.H)/float64(l.rect.W) >= 1.25 {
		splitH = true
	}

	if splitH {
		span := l.rect.H - minSize
		if span <= minSize {
			return false
		}
		s := rng.Intn(span-minSize) + minSize
		l.left = &bspLeaf{rect: Rect{l.rect.X, l.rect.Y, l.rect.W, s}}
		l.right = &bspLeaf{rect: Rect{l.rect.X, l.rect.Y + s, l.rect.W, l.rect.H - s}}
	} else {
		span := l.rect.W - minSize
		if span <= minSize {
			return false
		}
		s := rng.Intn(span-minSize) + minSize
		l.left = &bspLeaf{rect: Rect{l.rect.X, l.rect.Y, s, l.rect.H}}
		l.right = &bspLeaf{rect: Rect{l.rect.X + s, l.rect.Y, l.rect.W - s, l.rect.H}}
	}
	return true
}

func (l *bspLeaf) createRooms(rng *rand.Rand) {
	if l.left != nil || l.right != nil {
		if l.left != nil {
			l.left.createRooms(rng)
		}
		if l.right != nil {
			l.right.createRooms(rng)
		}
		return
	}
	const margin = 1
	minW, minH := 4, 4
	maxW := l.rect.W - margin*2
	maxH := l.rect.H - margin*2
	if maxW < minW || maxH < minH {
		return
	}
	w := minW + rng.Intn(maxW-minW+1)
	h := minH + rng.Intn(maxH-minH+1)
	x := l.rect.X + margin + rng.Intn(max(1, l.rect.W-margin*2-w+1))
	y := l.rect.Y + margin + rng.Intn(max(1, l.rect.H-margin*2-h+1))
	l.room = &Rect{x, y, w, h}
}

// getRoom returns the room in this subtree (DFS, prefers left).
func (l *bspLeaf) getRoom() *Rect {
	if l.room != nil {
		return l.room
	}
	if l.left != nil {
		if r := l.left.getRoom(); r != nil {
			return r
		}
	}
	if l.right != nil {
		return l.right.getRoom()
	}
	return nil
}

func (l *bspLeaf) connectRooms(d *Dungeon) {
	if l.left == nil || l.right == nil {
		return
	}
	l.left.connectRooms(d)
	l.right.connectRooms(d)

	r1 := l.left.getRoom()
	r2 := l.right.getRoom()
	if r1 == nil || r2 == nil {
		return
	}
	// L-shaped corridor: horizontal from r1 centre, then vertical to r2 centre.
	x1, y1 := r1.CenterX(), r1.CenterY()
	x2, y2 := r2.CenterX(), r2.CenterY()
	d.hCorridor(x1, x2, y1)
	d.vCorridor(y1, y2, x2)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// mobLevel computes a mob level based on how far the room is from spawn.
// Rooms near spawn: level 1-2. Rooms far away: up to level 8.
// 5 % chance of an elite mob that is 5 levels higher (capped at 15).
func mobLevel(roomIdx, totalRooms int, rng *rand.Rand) uint32 {
	depth := float64(roomIdx) / float64(max(totalRooms-1, 1))
	base := int(depth*7) + 1 // 1 at spawn-adjacent, 8 at furthest room
	base += rng.Intn(3) - 1  // ±1 random variation
	if base < 1 {
		base = 1
	}
	if base > 10 {
		base = 10
	}
	if rng.Intn(100) < 5 { // elite
		base += 5
		if base > 15 {
			base = 15
		}
	}
	return uint32(base)
}
