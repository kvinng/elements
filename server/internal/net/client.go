package net

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kving/games/elements/server/internal/entity"
	"github.com/kving/games/elements/server/internal/world"
)

const writeTimeout = 5 * time.Second

// clientMsg is any message the client sends to the server.
type clientMsg struct {
	Type  string  `json:"type"`   // "input" | "chat"
	Seq   uint32  `json:"seq"`
	MoveX float32 `json:"move_x"`
	MoveY float32 `json:"move_y"`
	Fire  bool    `json:"fire"`
	AimX  float32 `json:"aim_x"`
	AimY  float32 `json:"aim_y"`
	Text  string  `json:"text"`
}

// Client represents one connected player.
type Client struct {
	entityID entity.EntityID
	name     string
	conn     *websocket.Conn
	send     chan []byte // pre-marshalled JSON from the hub
	hub      *Hub
}

// writePump sends the welcome message and then relays pre-marshalled frames.
// Exits when the hub closes the send channel.
func (c *Client) writePump() {
	defer c.conn.Close()

	welcome, _ := json.Marshal(serverMsg{
		Type:     "welcome",
		EntityID: c.entityID,
		Name:     c.name,
	})
	c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err := c.conn.WriteMessage(websocket.TextMessage, welcome); err != nil {
		return
	}

	for data := range c.send {
		c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

// readPump reads frames from the WebSocket and routes them by type.
// Triggers unregister when the connection closes.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("client read error", "entity_id", c.entityID, "err", err)
			}
			return
		}
		var msg clientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "chat":
			text := strings.TrimSpace(msg.Text)
			if text == "" {
				continue
			}
			select {
			case c.hub.chatCh <- ChatEvent{EntityID: c.entityID, Name: c.name, Text: text}:
			default:
			}
		default: // "input" or missing type (backwards compat)
			select {
			case c.hub.zone.InputCh <- world.InputEvent{
				Owner: c.entityID,
				State: entity.InputState{
					Seq:   msg.Seq,
					MoveX: msg.MoveX,
					MoveY: msg.MoveY,
					Fire:  msg.Fire,
					AimX:  msg.AimX,
					AimY:  msg.AimY,
				},
			}:
			default:
			}
		}
	}
}
