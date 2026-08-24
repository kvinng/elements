## Game.gd — Escena principal de juego: input loop, cámara, entidades y chat.
extends Node2D

const TILE_SIZE    := 32
const PLAYER_R     := 14.0
const INPUT_HZ     := 20.0        # igual que el servidor

# Colores por elemento (índice 0-4: None, Fire, Water, Earth, Air)
const EL_COLOR := [
	Color(0.753, 0.518, 0.988),   # None  — púrpura
	Color(0.976, 0.451, 0.086),   # Fire  — naranja
	Color(0.376, 0.647, 0.980),   # Water — azul
	Color(0.639, 0.898, 0.208),   # Earth — verde lima
	Color(0.886, 0.910, 0.941),   # Air   — blanco
]
const EL_NAME := ["No-Maestro", "Fuego", "Agua", "Tierra", "Aire"]

# ── Nodos ─────────────────────────────────────────────────────────────────────
@onready var camera:       Camera2D  = $Camera
@onready var dungeon_root: Node2D    = $Dungeon
@onready var entity_root:  Node2D    = $Entities
@onready var hud:          CanvasLayer = $HUD
@onready var hp_label:     Label     = $HUD/Stats/HPLabel
@onready var el_label:     Label     = $HUD/Stats/ElLabel
@onready var tick_label:   Label     = $HUD/Stats/TickLabel
@onready var chat_log:     RichTextLabel = $HUD/Chat/Log
@onready var chat_input:   LineEdit  = $HUD/Chat/Input

# ── Estado ────────────────────────────────────────────────────────────────────
var _my_id:    int = -1
var _entities: Dictionary = {}   # id → EntityNode2D
var _firing:   bool = false
var _input_timer: float = 0.0
var _dungeon_image: Image     # para la imagen de tiles precalculada
var _dungeon_texture: ImageTexture
var _dungeon_sprite: Sprite2D

func _ready() -> void:
	Network.connected.connect(_on_connected)
	Network.snapshot_received.connect(_on_snapshot)
	Network.map_received.connect(_on_map)
	Network.chat_received.connect(_on_chat)
	Network.disconnected.connect(_on_disconnected)

	chat_input.text_submitted.connect(_send_chat)

func _on_connected(entity_id: int, player_name: String) -> void:
	_my_id = entity_id
	_add_chat_line("Sistema", "Conectado como %s (#%d)" % [player_name, entity_id])

func _on_disconnected() -> void:
	_add_chat_line("Sistema", "Desconectado del servidor.")

# ── Mapa del dungeon ──────────────────────────────────────────────────────────

func _on_map(data: Dictionary) -> void:
	var w: int = data.get("map_width", 0)
	var h: int = data.get("map_height", 0)
	var ts: int = data.get("tile_size", TILE_SIZE)
	var b64: String = data.get("tiles", "")
	if w == 0 or b64.is_empty():
		return

	var raw: PackedByteArray = Marshalls.base64_to_raw(b64)

	# Dibuja el dungeon en una Image y luego la convierte a Sprite2D
	_dungeon_image = Image.create(w * ts, h * ts, false, Image.FORMAT_RGB8)

	var wall_dark  := Color(0.050, 0.067, 0.090)
	var wall_mid   := Color(0.176, 0.216, 0.282)
	var wall_light := Color(0.216, 0.255, 0.318)
	var floor_dark := Color(0.102, 0.122, 0.188)
	var floor_mid  := Color(0.118, 0.141, 0.220)

	for ty in range(h):
		for tx in range(w):
			var tile := raw[ty * w + tx]
			var px := tx * ts
			var py := ty * ts
			if tile == 2:   # floor
				_dungeon_image.fill_rect(Rect2i(px, py, ts, ts), floor_dark)
				_dungeon_image.fill_rect(Rect2i(px+1, py+1, ts-2, ts-2), floor_mid)
			elif tile == 1: # wall
				_dungeon_image.fill_rect(Rect2i(px, py, ts, ts), wall_dark)
				_dungeon_image.fill_rect(Rect2i(px, py, ts-1, ts-1), wall_mid)
				_dungeon_image.fill_rect(Rect2i(px+2, py+2, ts-5, ts-5), wall_light)

	_dungeon_texture = ImageTexture.create_from_image(_dungeon_image)
	_dungeon_sprite = Sprite2D.new()
	_dungeon_sprite.texture = _dungeon_texture
	_dungeon_sprite.centered = false
	dungeon_root.add_child(_dungeon_sprite)

# ── Snapshot ──────────────────────────────────────────────────────────────────

func _on_snapshot(entities: Array) -> void:
	var seen := {}
	for e in entities:
		var id: int = int(e.get("id", 0))
		seen[id] = true

		if not _entities.has(id):
			_entities[id] = _create_entity_node(id)

		_update_entity_node(_entities[id], e)

	# Elimina entidades que ya no están
	for id in _entities.keys():
		if not seen.has(id):
			_entities[id].queue_free()
			_entities.erase(id)

	# Centra la cámara en el jugador propio
	if _my_id != -1 and _entities.has(_my_id):
		camera.global_position = _entities[_my_id].global_position

func _create_entity_node(id: int) -> Node2D:
	var node := Node2D.new()
	node.set_meta("eid", id)
	entity_root.add_child(node)
	return node

func _update_entity_node(node: Node2D, e: Dictionary) -> void:
	var kind:    int = int(e.get("kind", 0))
	var el_idx:  int = int(e.get("element", 0))
	var hp:      int = int(e.get("hp", 0))
	var max_hp:  int = int(e.get("max_hp", 100))
	var name_s:  String = e.get("name", "")
	var is_me:   bool = (int(e.get("id", -1)) == _my_id)

	node.global_position = Vector2(float(e.get("x", 0)), float(e.get("y", 0)))
	node.queue_redraw()

	# Guarda datos para el draw
	node.set_meta("kind", kind)
	node.set_meta("el", el_idx)
	node.set_meta("hp", hp)
	node.set_meta("max_hp", max_hp)
	node.set_meta("ename", name_s)
	node.set_meta("is_me", is_me)

	if not node.draw.is_connected(_draw_entity.bind(node)):
		node.draw.connect(_draw_entity.bind(node))

	# HUD del jugador propio
	if is_me:
		hp_label.text = "HP: %d/%d" % [hp, max_hp]
		el_label.text = EL_NAME[el_idx] if el_idx < EL_NAME.size() else "?"
		el_label.modulate = EL_COLOR[el_idx] if el_idx < EL_COLOR.size() else Color.WHITE

func _draw_entity(node: Node2D) -> void:
	var kind:   int    = node.get_meta("kind", 0)
	var el:     int    = node.get_meta("el", 0)
	var hp:     int    = node.get_meta("hp", 0)
	var max_hp: int    = node.get_meta("max_hp", 100)
	var ename:  String = node.get_meta("ename", "")
	var is_me:  bool   = node.get_meta("is_me", false)
	var color := EL_COLOR[el] if el < EL_COLOR.size() else Color.WHITE

	match kind:
		0: # Jugador
			var R := PLAYER_R
			var outline_color := Color.WHITE if is_me else Color(1,1,1,0.35)
			node.draw_circle(Vector2.ZERO, R + 10, Color(color, 0.2))
			node.draw_circle(Vector2(1, 2), R, Color(0, 0, 0, 0.4))
			node.draw_circle(Vector2.ZERO, R, color)
			node.draw_arc(Vector2.ZERO, R, 0, TAU, 32, outline_color, 2.0 if is_me else 1.5)
			if hp <= 0:
				node.draw_circle(Vector2.ZERO, R, Color(0, 0, 0, 0.65))
			else:
				_draw_hp_bar(node, R, hp, max_hp)
				var label := ename + " [" + (EL_NAME[el] if el < EL_NAME.size() else "?") + "]"
				# Nombre (usando draw_string con fuente por defecto)
				var font := ThemeDB.fallback_font
				node.draw_string(font, Vector2(-30, -R - 14), label, HORIZONTAL_ALIGNMENT_LEFT, -1, 9, Color.WHITE if is_me else color)

		1: # Proyectil
			var R := 5.0 if el != 3 else 8.0
			node.draw_circle(Vector2.ZERO, R + 5, Color(color, 0.3))
			node.draw_circle(Vector2.ZERO, R, color)

		2: # Mob — diamante
			var R := 11.0
			var pts := PackedVector2Array([
				Vector2(0, -R), Vector2(R, 0),
				Vector2(0, R),  Vector2(-R, 0),
			])
			node.draw_colored_polygon(pts, Color(color, 0.8))
			node.draw_polyline(PackedVector2Array([pts[0], pts[1], pts[2], pts[3], pts[0]]), color, 2.0)
			_draw_hp_bar(node, R, hp, max_hp)

		3: # Item — orbe verde
			var t := Time.get_ticks_msec() / 600.0
			var bob := sin(t + float(node.get_meta("eid", 0))) * 3.0
			var pos := Vector2(0, bob)
			node.draw_circle(pos, 12, Color(0.086, 0.396, 0.204, 0.5))
			node.draw_circle(pos, 8, Color(0.102, 0.396, 0.204))
			node.draw_arc(pos, 8, 0, TAU, 16, Color(0.302, 0.937, 0.502), 2.0)

func _draw_hp_bar(node: Node2D, R: float, hp: int, max_hp: int) -> void:
	if max_hp <= 0:
		return
	var bw := 38.0; var bh := 4.0
	var by := -R - 10.0
	var ratio := clampf(float(hp) / float(max_hp), 0.0, 1.0)
	var bar_color := Color(0.290, 0.867, 0.502) if ratio > 0.5 else \
	                 Color(0.980, 0.800, 0.082) if ratio > 0.25 else \
	                 Color(0.973, 0.529, 0.529)
	node.draw_rect(Rect2(-bw/2, by, bw, bh), Color(0.118, 0.180, 0.235))
	node.draw_rect(Rect2(-bw/2, by, bw * ratio, bh), bar_color)
	node.draw_rect(Rect2(-bw/2, by, bw, bh), Color(0.216, 0.255, 0.318), false, 1.0)

# ── Input ─────────────────────────────────────────────────────────────────────

func _process(delta: float) -> void:
	_input_timer += delta
	if _input_timer < 1.0 / INPUT_HZ:
		return
	_input_timer = 0.0

	if chat_input.has_focus():
		Network.send_input(0, 0, false, 0, 1)
		return

	var mx := Input.get_axis("move_left", "move_right")
	var my := Input.get_axis("move_up", "move_down")
	_firing = Input.is_action_pressed("fire")

	# Dirección de apuntado: ratón relativo al jugador en mundo
	var aim := Vector2.DOWN
	if _my_id != -1 and _entities.has(_my_id):
		var me_pos := _entities[_my_id].global_position
		var mouse_world := get_viewport().get_canvas_transform().affine_inverse() * get_viewport().get_mouse_position()
		aim = (mouse_world - me_pos).normalized()

	Network.send_input(mx, my, _firing, aim.x, aim.y)

	# Actualiza tick en HUD
	tick_label.text = "Tick: ?"

func _input(event: InputEvent) -> void:
	if event.is_action_pressed("chat") and not chat_input.has_focus():
		chat_input.grab_focus()
		get_viewport().set_input_as_handled()

# ── Chat ──────────────────────────────────────────────────────────────────────

func _send_chat(text: String) -> void:
	text = text.strip_edges()
	if not text.is_empty():
		Network.send_chat(text)
	chat_input.text = ""
	chat_input.release_focus()

func _on_chat(entity_id: int, name_s: String, text: String) -> void:
	_add_chat_line(name_s, text)

func _add_chat_line(speaker: String, text: String) -> void:
	chat_log.append_text("[color=#93c5fd]%s:[/color] %s\n" % [speaker, text])
	# Scroll al final
	await get_tree().process_frame
	chat_log.scroll_to_line(chat_log.get_line_count() - 1)
