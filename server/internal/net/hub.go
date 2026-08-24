package net

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/kving/games/elements/server/internal/entity"
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
	zone       *world.World
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	chatCh     chan ChatEvent
}

func NewHub(zone *world.World) *Hub {
	return &Hub{
		zone:       zone,
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client, 8),
		unregister: make(chan *Client, 8),
		chatCh:     make(chan ChatEvent, 32),
	}
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
			slog.Info("client joined", "entity_id", c.entityID, "name", c.name, "total", len(h.clients))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				h.zone.DespawnCh <- c.entityID
				slog.Info("client left", "entity_id", c.entityID, "name", c.name, "total", len(h.clients))
			}

		case snap := <-h.zone.SnapshotCh:
			// Marshal once, fan out to all clients.
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
			close(c.send)
			h.zone.DespawnCh <- c.entityID
		}
	}
}

// parseElement maps a query-string value to an ElementType.
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

// ServeWS upgrades the HTTP connection and registers the client with the hub.
// Reads ?name= and ?element= from the query string.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("name")
	if name == "" {
		name = "Anon"
	}
	el := parseElement(q.Get("element"))

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}

	result := make(chan entity.EntityID, 1)
	h.zone.SpawnCh <- world.SpawnRequest{
		Pos:    entity.Position{X: world.RespawnX, Y: world.RespawnY},
		HP:     100,
		El:     entity.Element{Kind: el, Level: 1},
		Name:   name,
		Result: result,
	}
	id := <-result

	c := &Client{
		entityID: id,
		name:     name,
		conn:     conn,
		send:     make(chan []byte, 16),
		hub:      h,
	}
	h.register <- c

	go c.writePump()
	go c.readPump()
}
