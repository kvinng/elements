package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Type     string `json:"type"`
	EntityID uint64 `json:"entity_id"`
	Name     string `json:"name"`
	Text     string `json:"text"`
	Tick     uint64 `json:"tick"`
	Entities []struct {
		ID      uint64  `json:"id"`
		X       float32 `json:"x"`
		Y       float32 `json:"y"`
		HP      int32   `json:"hp"`
		MaxHP   int32   `json:"max_hp"`
		Name    string  `json:"name"`
		Kind    uint8   `json:"kind"`
		Element uint8   `json:"element"`
	} `json:"entities"`
}

var elementNames = [5]string{"None", "Fire", "Water", "Earth", "Air"}

func client(name, element string, mx, my, aimX, aimY float32, fire bool, done <-chan struct{}) {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws?name="+name+"&element="+element, nil)
	if err != nil {
		fmt.Println(name, "dial error:", err)
		return
	}
	defer conn.Close()

	var myID uint64
	go func() {
		for {
			_, d, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m Msg
			json.Unmarshal(d, &m) //nolint:errcheck
			switch m.Type {
			case "welcome":
				myID = m.EntityID
				fmt.Printf("[%s] conectado como #%d\n", name, myID)
			case "chat":
				fmt.Printf("[CHAT] %s: %s\n", m.Name, m.Text)
			case "snapshot":
				if m.Tick%20 != 0 {
					break
				}
				for _, e := range m.Entities {
					tag := ""
					if e.ID == myID {
						tag = " ◄"
					}
					kind := "player"
					if e.Kind == 1 {
						kind = "proj "
					}
					elName := elementNames[e.Element]
					fmt.Printf("  [%-5s] tick=%-5d %s #%-2d %-8s [%-5s] x=%-7.0f y=%-7.0f hp=%d/%d%s\n",
						name, m.Tick, kind, e.ID, e.Name, elName, e.X, e.Y, e.HP, e.MaxHP, tag)
				}
			}
		}
	}()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var seq uint32
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			seq++
			msg := fmt.Sprintf(
				`{"type":"input","seq":%d,"move_x":%g,"move_y":%g,"fire":%v,"aim_x":%g,"aim_y":%g}`,
				seq, mx, my, fire, aimX, aimY)
			conn.WriteMessage(websocket.TextMessage, []byte(msg)) //nolint:errcheck
		}
	}
}

func sendChat(name, text string) {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws?name="+name, nil)
	if err != nil {
		return
	}
	time.Sleep(200 * time.Millisecond) // wait for welcome
	conn.WriteMessage(websocket.TextMessage, //nolint:errcheck
		[]byte(fmt.Sprintf(`{"type":"chat","text":%q}`, text)))
	time.Sleep(100 * time.Millisecond)
	conn.Close()
}

func main() {
	done := make(chan struct{})

	// Aang (Air): fast, low HP, shoots right toward Zuko
	go client("Aang", "air", 0, 0, 1, 0, true, done)
	time.Sleep(100 * time.Millisecond)

	// Zuko (Fire): balanced, shoots left toward Aang
	// Fire beats Air → Zuko's shots do 1.5× to Aang; Aang's do 0.7× to Zuko
	go client("Zuko", "fire", 0, 0, -1, 0, true, done)

	// Chat message after 1 second
	time.Sleep(1 * time.Second)
	go sendChat("Sokka", "Hola desde el servidor!")

	time.Sleep(4 * time.Second)
	close(done)
	time.Sleep(100 * time.Millisecond)
}
