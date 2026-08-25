## Network.gd — Autoload singleton. Gestiona auth REST y la conexión WebSocket.
extends Node

signal auth_success(player_data: Dictionary)
signal auth_failed(error: String)
signal connected(entity_id: int, player_name: String)
signal snapshot_received(entities: Array)
signal map_received(data: Dictionary)
signal chat_received(entity_id: int, player_name: String, text: String)
signal disconnected()

const BASE_URL := "http://localhost:8080"
const WS_URL   := "ws://localhost:8080/ws"

# Mapeo error_code del servidor → clave de tr().
# Añadir aquí cualquier código nuevo que el servidor devuelva.
const ERROR_CODE_KEY := {
	"name_taken":       "ERR_NAME_TAKEN",
	"bad_credentials":  "ERR_BAD_CREDENTIALS",
	"invalid_request":  "ERR_INVALID_REQUEST",
	"internal_error":   "ERR_INTERNAL_ERROR",
}

var _ws    := WebSocketPeer.new()
var _state := WebSocketPeer.STATE_CLOSED
var _seq   := 0
var _token := ""

# ── Auth REST ─────────────────────────────────────────────────────────────────

func login(player_name: String, password: String) -> void:
	_post("/api/auth/login",
		{"name": player_name, "password": password},
		_on_auth_response)

func register(player_name: String, password: String, element: String) -> void:
	_post("/api/auth/register",
		{"name": player_name, "password": password, "element": element},
		_on_auth_response)

func _post(path: String, body: Dictionary, callback: Callable) -> void:
	var http := HTTPRequest.new()
	add_child(http)
	http.request_completed.connect(callback.bind(http))
	var err := http.request(
		BASE_URL + path,
		["Content-Type: application/json"],
		HTTPClient.METHOD_POST,
		JSON.stringify(body)
	)
	if err != OK:
		http.queue_free()
		emit_signal("auth_failed", tr("ERR_CONNECT_FAILED"))

func _on_auth_response(result: int, response_code: int, _headers: PackedStringArray,
		body: PackedByteArray, http: HTTPRequest) -> void:
	http.queue_free()
	if result != HTTPRequest.RESULT_SUCCESS:
		emit_signal("auth_failed", tr("ERR_NETWORK"))
		return
	var data = JSON.parse_string(body.get_string_from_utf8())
	if data == null:
		emit_signal("auth_failed", tr("ERR_INVALID_RESPONSE"))
		return
	if response_code >= 400:
		var code: String = data.get("error_code", "")
		var tr_key: String = ERROR_CODE_KEY.get(code, "ERR_UNKNOWN")
		emit_signal("auth_failed", tr(tr_key))
		return
	_token = data.get("token", "")
	emit_signal("auth_success", data)

# ── WebSocket ─────────────────────────────────────────────────────────────────

func connect_ws() -> void:
	if _token.is_empty():
		push_error("Network: connect_ws llamado sin token")
		return
	var err = _ws.connect_to_url(WS_URL + "?token=" + _token.uri_encode())
	if err != OK:
		push_error("WebSocket connect error: %d" % err)

func disconnect_from_server() -> void:
	_ws.close()

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
		"error":
			push_error("Server error: " + msg.get("text", ""))

# ── Envío ─────────────────────────────────────────────────────────────────────

func send_input(move_x: float, move_y: float, firing: bool, aim_x: float, aim_y: float) -> void:
	if _ws.get_ready_state() != WebSocketPeer.STATE_OPEN:
		return
	_seq += 1
	_ws.send_text(JSON.stringify({
		"type": "input", "seq": _seq,
		"move_x": move_x, "move_y": move_y,
		"fire": firing,
		"aim_x": aim_x, "aim_y": aim_y,
	}))

func send_chat(text: String) -> void:
	if _ws.get_ready_state() != WebSocketPeer.STATE_OPEN:
		return
	_ws.send_text(JSON.stringify({"type": "chat", "text": text}))
