package net

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/kving/games/elements/server/internal/auth"
	"github.com/kving/games/elements/server/internal/dungeon"
	"github.com/kving/games/elements/server/internal/entity"
	"github.com/kving/games/elements/server/internal/store"
	"github.com/kving/games/elements/server/internal/world"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// serverMsg is the envelope for all server → client messages.
type serverMsg struct {
	Type     string                 `json:"type"`
	Tick     uint64                 `json:"tick,omitempty"`
	EntityID entity.EntityID        `json:"entity_id,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Text     string                 `json:"text,omitempty"`
	Entities []world.EntitySnapshot `json:"entities,omitempty"`
	// map fields
	MapWidth  int     `json:"map_width,omitempty"`
	MapHeight int     `json:"map_height,omitempty"`
	TileSize  int     `json:"tile_size,omitempty"`
	Tiles     []byte  `json:"tiles,omitempty"`
	SpawnX    float32 `json:"spawn_x,omitempty"`
	SpawnY    float32 `json:"spawn_y,omitempty"`
	// progression (welcome message)
	Level  uint32 `json:"level,omitempty"`
	XP     uint32 `json:"xp,omitempty"`
	XPNext uint32 `json:"xp_next,omitempty"`
}

// ChatEvent is a chat message from one client to be broadcast to all.
type ChatEvent struct {
	EntityID entity.EntityID
	Name     string
	Text     string
}

// Hub manages all connected WebSocket clients for a single zone.
// It runs in one goroutine and owns the clients map without any locking.
type Hub struct {
	zone          *world.World
	store         store.PlayerStore
	authSvc       *auth.Service
	clients       map[*Client]struct{}
	entityClients map[entity.EntityID]*Client // reverse lookup for snapshot updates
	register      chan *Client
	unregister    chan *Client
	chatCh        chan ChatEvent
	mapBytes      []byte // pre-encoded map message, sent once per client on connect
}

func NewHub(zone *world.World, st store.PlayerStore, authSvc *auth.Service) *Hub {
	h := &Hub{
		zone:          zone,
		store:         st,
		authSvc:       authSvc,
		clients:       make(map[*Client]struct{}),
		entityClients: make(map[entity.EntityID]*Client),
		register:      make(chan *Client, 8),
		unregister:    make(chan *Client, 8),
		chatCh:        make(chan ChatEvent, 32),
	}
	if zone.Dungeon != nil {
		h.mapBytes = buildMapMsg(zone.Dungeon)
	}
	return h
}

func buildMapMsg(d *dungeon.Dungeon) []byte {
	flat := make([]byte, d.Width*d.Height)
	for y, row := range d.Tiles {
		for x, t := range row {
			flat[y*d.Width+x] = byte(t)
		}
	}
	data, _ := json.Marshal(serverMsg{
		Type:      "map",
		MapWidth:  d.Width,
		MapHeight: d.Height,
		TileSize:  dungeon.TileSize,
		Tiles:     flat,
		SpawnX:    d.PlayerSpawn[0],
		SpawnY:    d.PlayerSpawn[1],
	})
	return data
}

// Run is the hub event loop. It must run in its own goroutine.
func (h *Hub) Run(ctx context.Context) {
	slog.Info("hub started")
	for {
		select {
		case <-ctx.Done():
			return

		case c := <-h.register:
			h.clients[c] = struct{}{}
			h.entityClients[c.entityID] = c
			slog.Info("client joined", "entity_id", c.entityID, "name", c.name, "total", len(h.clients))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				delete(h.entityClients, c.entityID)
				close(c.send)
				h.zone.DespawnCh <- c.entityID
				// Persist progression asynchronously so we don't block the hub loop.
				go h.store.Save(context.Background(), c.playerID, c.level, c.xp) //nolint:errcheck
				slog.Info("client left", "entity_id", c.entityID, "name", c.name, "total", len(h.clients))
			}

		case snap := <-h.zone.SnapshotCh:
			// Cache the latest level/xp for each connected player so we can
			// save accurate data on disconnect.
			for _, e := range snap.Entities {
				if c, ok := h.entityClients[e.ID]; ok {
					c.level = e.Level
					c.xp = e.XP
				}
			}
			data, err := json.Marshal(serverMsg{
				Type:     "snapshot",
				Tick:     snap.Tick,
				Entities: snap.Entities,
			})
			if err != nil {
				continue
			}
			h.broadcast(data)

		case ev := <-h.chatCh:
			data, err := json.Marshal(serverMsg{
				Type:     "chat",
				EntityID: ev.EntityID,
				Name:     ev.Name,
				Text:     ev.Text,
			})
			if err != nil {
				continue
			}
			h.broadcast(data)
		}
	}
}

// broadcast sends data to every connected client, disconnecting slow ones.
func (h *Hub) broadcast(data []byte) {
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			delete(h.clients, c)
			delete(h.entityClients, c.entityID)
			close(c.send)
			h.zone.DespawnCh <- c.entityID
		}
	}
}

// parseElement maps a string to an ElementType.
func parseElement(s string) entity.ElementType {
	switch s {
	case "fire":
		return entity.ElementFire
	case "water":
		return entity.ElementWater
	case "earth":
		return entity.ElementEarth
	case "air":
		return entity.ElementAir
	default:
		return entity.ElementNone
	}
}

// ServeWS upgrades the HTTP connection, validates the JWT from ?token=,
// loads fresh player state from DB, then registers the client and starts I/O pumps.
//
// Auth flow:
//
//	Client          REST /api/auth/login → { token, name, element, level, xp }
//	Client          GET /ws?token=<jwt>
//	ServeWS         validate token → playerID → store.GetByID → spawn → welcome
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	// ── Token validation (before upgrade — returns plain HTTP errors) ─────────
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	playerID, err := h.authSvc.Validate(tokenStr)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Always load fresh state from DB — level/xp change during gameplay and
	// may be stale in the token (especially with multiple game servers sharing one DB).
	player, err := h.store.GetByID(r.Context(), store.PlayerID(playerID))
	if err != nil {
		http.Error(w, "player not found", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}

	// ── Spawn ─────────────────────────────────────────────────────────────────
	elType := entity.ElementType(player.Element)
	spawnX, spawnY := world.RespawnX, world.RespawnY
	if h.zone.Dungeon != nil {
		spawnX = h.zone.Dungeon.PlayerSpawn[0]
		spawnY = h.zone.Dungeon.PlayerSpawn[1]
	}
	result := make(chan entity.EntityID, 1)
	h.zone.SpawnCh <- world.SpawnRequest{
		Pos:    entity.Position{X: spawnX, Y: spawnY},
		HP:     world.BaseHP(elType, player.Level),
		El:     entity.Element{Kind: elType, Level: 1},
		Name:   player.Name,
		Level:  player.Level,
		XP:     player.XP,
		Result: result,
	}
	id := <-result

	c := &Client{
		entityID: id,
		playerID: player.ID,
		name:     player.Name,
		level:    player.Level,
		xp:       player.XP,
		conn:     conn,
		send:     make(chan []byte, 16),
		hub:      h,
	}
	h.register <- c

	go c.writePump()
	go c.readPump()
}

