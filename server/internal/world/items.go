package world

import "github.com/kving/games/elements/server/internal/entity"

const (
	pickupRange    float32 = 20 // units: auto-collect when this close
	healthRestore  int32   = 40 // HP gained from a health item
)

// systemItems checks whether any player is close enough to pick up a floor item.
func systemItems(w *World) {
	var collected []entity.EntityID

	for itemID, item := range w.items {
		ipos := w.positions[itemID]
		for pid := range w.inputs {
			ppos := w.positions[pid]
			if magnitude(ppos.X-ipos.X, ppos.Y-ipos.Y) > pickupRange {
				continue
			}
			// Apply effect.
			switch item.Kind {
			case entity.ItemHealth:
				h := w.healths[pid]
				h.Current += healthRestore
				if h.Current > h.Max {
					h.Current = h.Max
				}
				w.healths[pid] = h
			case entity.ItemGold:
				w.golds[pid] += item.Amount
			}
			collected = append(collected, itemID)
			break // one player picks up this item
		}
	}

	for _, id := range collected {
		delete(w.positions, id)
		delete(w.items, id)
	}
}
