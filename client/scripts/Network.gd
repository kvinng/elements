## Network.gd — Autoload singleton que gestiona la conexión WebSocket con el servidor Go.
## Emite señales que el resto de escenas escuchan; nunca bloquea.
extends Node

signal connected(entity_id: int, name: String)
signal snapshot_received(entities: Array)
signal map_received(data: Dictionary)
signal chat_received(entity_id: int, name: String, text: String)
signal disconnected()

const SERVER_URL := "ws://localhost:8080/ws"

var _ws := WebSocketPeer.new()
var _state := WebSocketPeer.STATE_CLOSED
var _seq := 0

# Conecta al servidor con nombre y elemento elegido en el lobby.
func connect_to_server(player_name: String, element: String) -> void:
	var url = "%s?name=%s&element=%s" % [SERVER_URL, player_name.uri_encode(), element]
	var err = _ws.connect_to_url(url)
	if err != OK:
		push_error("WebSocket connect error: %d" % err)

func disconnect_from_server() -> void:
	_ws.close()

# Llamar cada frame para procesar mensajes entrantes.
func _process(_delta: float) -> void:
	_ws.poll()
	var new_state = _ws.get_ready_state()

	match new_state:
		WebSocketPeer.STATE_OPEN:
			while _ws.get_available_packet_count() > 0:
				_handle_packet(_ws.get_packet())
		WebSocketPeer.STATE_CLOSED:
			if _state != WebSocketPeer.STATE_CLOSED:
				emit_signal("disconnected")

	_state = new_state

func _handle_packet(raw: PackedByteArray) -> void:
	var text := raw.get_string_from_utf8()
	var msg = JSON.parse_string(text)
	if msg == null:
		return

	match msg.get("type", ""):
		"welcome":
			emit_signal("connected", int(msg.get("entity_id", 0)), msg.get("name", ""))
		"map":
			emit_signal("map_received", msg)
		"snapshot":
			emit_signal("snapshot_received", msg.get("entities", []))
		"chat":
			emit_signal("chat_received",
				int(msg.get("entity_id", 0)),
				msg.get("name", ""),
				msg.get("text", ""))

# ── Envío de mensajes ─────────────────────────────────────────────────────────

func send_input(move_x: float, move_y: float, firing: bool, aim_x: float, aim_y: float) -> void:
	if _ws.get_ready_state() != WebSocketPeer.STATE_OPEN:
		return
	_seq += 1
	var msg := {
		"type": "input", "seq": _seq,
		"move_x": move_x, "move_y": move_y,
		"fire": firing,
		"aim_x": aim_x, "aim_y": aim_y,
	}
	_ws.send_text(JSON.stringify(msg))

func send_chat(text: String) -> void:
	if _ws.get_ready_state() != WebSocketPeer.STATE_OPEN:
		return
	_ws.send_text(JSON.stringify({"type": "chat", "text": text}))
