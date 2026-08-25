## Game.gd — Dungeon tiles + sprites animados + input + chat.
extends Node2D

const TILE_SIZE    := 32
const PLAYER_R     := 14.0
const INPUT_HZ     := 20.0
const SPR_SCALE    := Vector2(0.75, 0.75)   # 64px sprite → 48px en mundo
const DUNGEON_PATH := "res://assets/dungeon/"

const EL_COLOR := [
	Color(0.753, 0.518, 0.988),   # None  — púrpura
	Color(0.976, 0.451, 0.086),   # Fire  — naranja
	Color(0.376, 0.647, 0.980),   # Water — azul
	Color(0.639, 0.898, 0.208),   # Earth — verde lima
	Color(0.886, 0.910, 0.941),   # Air   — blanco
]
# Claves de traducción indexadas por ElementType (0=None … 4=Air).
const EL_NAME_KEY := ["EL_NONE", "EL_FIRE", "EL_WATER", "EL_EARTH", "EL_AIR"]

func _el_name(el: int) -> String:
	if el < EL_NAME_KEY.size():
		return tr(EL_NAME_KEY[el])
	return "?"

# Bitmask de 4 vecinos → nombre semántico de tile.
# Bits: N=1  E=2  S=4  W=8  (bit activo = ese vecino es muro)
# El nombre se resuelve contra el JSON del tileset activo en _on_map().
# Cambiar tileset = cambiar el JSON; estos nombres no cambian.
const WALL_BITMASK_NAMES := [
	"pared_top",             #  0: aislado           → top por defecto
	"pared_front",           #  1: N        → cara S visible
	"pared_front",           #  2: E        → cara S visible
	"pared_front",           #  3: N+E      → cara S visible
	"pared_top",             #  4: S        → cara N visible
	"pared_izq",             #  5: N+S      → pasillo vertical
	"esquina_top_izq",       #  6: E+S      → esquina NW
	"pared_der",             #  7: N+E+S    → borde oeste (pared der del corredor)
	"pared_front",           #  8: W        → cara S visible
	"pared_front",           #  9: N+W      → cara S visible
	"suelo_piedra_interior", # 10: E+W      → pasillo horizontal
	"pared_front",           # 11: N+E+W    → cara sur (el más frecuente)
	"esquina_top_der",       # 12: S+W      → esquina NE
	"pared_izq",             # 13: N+S+W    → borde este (pared izq del corredor)
	"pared_top",             # 14: E+S+W    → cara N visible
	"suelo_piedra_interior", # 15: rodeado  → interior oscuro
]

# Variantes de suelo: se alternan según posición+seed para evitar que el suelo
# sea uniforme. Añadir más nombres aquí si el tileset los tiene.
const FLOOR_VARIANTS := ["suelo_gris", "suelo_gris_decorativo"]

@onready var camera:       Camera2D      = $Camera
@onready var dungeon_root: Node2D        = $Dungeon
@onready var entity_root:  Node2D        = $Entities
@onready var hp_label:     Label         = $HUD/Stats/HPLabel
@onready var el_label:     Label         = $HUD/Stats/ElLabel
@onready var tick_label:   Label         = $HUD/Stats/TickLabel
@onready var lv_label:     Label         = $HUD/Stats/LvLabel
@onready var xp_bar_bg:    ColorRect     = $HUD/Stats/XPBarBG
@onready var xp_bar_fill:  ColorRect     = $HUD/Stats/XPBarBG/XPBarFill
@onready var chat_log:     RichTextLabel = $HUD/Chat/Log
@onready var chat_input:   LineEdit      = $HUD/Chat/Input

var _my_id: int = -1
var _entities: Dictionary = {}
var _firing: bool = false
var _input_timer: float = 0.0
var _el_power: float = 1.0        # 0..1, placeholder hasta que el server lo envíe

# SpriteFrames compartidos (buildeados una vez, reutilizados por todas las entidades)
var _frames_player: SpriteFrames
var _frames_mob:    SpriteFrames

# Barras de HUD creadas en código
var _hp_bar_bg:  ColorRect
var _hp_bar_fill: ColorRect
var _chi_bar_bg:  ColorRect
var _chi_bar_fill: ColorRect

func _ready() -> void:
	RenderingServer.set_default_clear_color(Color(0.04, 0.04, 0.08))
	Network.connected.connect(_on_connected)
	Network.snapshot_received.connect(_on_snapshot)
	Network.map_received.connect(_on_map)
	Network.chat_received.connect(_on_chat)
	Network.disconnected.connect(_on_disconnected)
	chat_input.text_submitted.connect(_send_chat)
	_build_sprite_frames()
	_build_hud_bars()

# ── Sprites ────────────────────────────────────────────────────────────────────

func _build_sprite_frames() -> void:
	var male_idle := load("res://assets/characters/male_idle.png") as Texture2D
	var male_walk := load("res://assets/characters/male_walk.png") as Texture2D
	var orc_idle  := load("res://assets/mobs/orc_idle.png")       as Texture2D
	var orc_walk  := load("res://assets/mobs/orc_walk.png")       as Texture2D
	if not male_idle or not male_walk or not orc_idle or not orc_walk:
		push_warning("Game: faltan texturas — cierra y reabre el proyecto en Godot para importarlas")
		return
	_frames_player = _make_frames(male_idle, 12, male_walk, 6)
	_frames_mob    = _make_frames(orc_idle,  4,  orc_walk,  6)

func _make_frames(idle_tex: Texture2D, idle_n: int, walk_tex: Texture2D, walk_n: int) -> SpriteFrames:
	# Spritesheets craftpix: n_frames columnas × 4 filas (sur/oeste/este/norte), 64×64 por frame.
	# Usamos fila 0 (sur/frontal) para el MVP. Para añadir 4 direcciones: crear una animación por fila.
	var sf := SpriteFrames.new()
	sf.rename_animation("default", "idle")
	sf.set_animation_speed("idle", 6.0)
	sf.set_animation_loop("idle", true)
	for col in range(idle_n):
		var at := AtlasTexture.new()
		at.atlas = idle_tex
		at.region = Rect2(col * 64, 0, 64, 64)
		sf.add_frame("idle", at)
	sf.add_animation("walk")
	sf.set_animation_speed("walk", 8.0)
	sf.set_animation_loop("walk", true)
	for col in range(walk_n):
		var at := AtlasTexture.new()
		at.atlas = walk_tex
		at.region = Rect2(col * 64, 0, 64, 64)
		sf.add_frame("walk", at)
	return sf

# ── HUD bars (creadas en código para no tocar la escena) ─────────────────────

func _build_hud_bars() -> void:
	var stats := $HUD/Stats

	# ── Barra de HP ───────────────────────────────────────────────────────────
	_hp_bar_bg = ColorRect.new()
	_hp_bar_bg.color = Color(0.12, 0.06, 0.06)
	_hp_bar_bg.custom_minimum_size = Vector2(160, 10)
	stats.add_child(_hp_bar_bg)

	_hp_bar_fill = ColorRect.new()
	_hp_bar_fill.color = Color(0.85, 0.18, 0.18)
	_hp_bar_fill.size = Vector2(160, 10)
	_hp_bar_bg.add_child(_hp_bar_fill)

	# ── Barra de Chi (energía elemental) ──────────────────────────────────────
	_chi_bar_bg = ColorRect.new()
	_chi_bar_bg.color = Color(0.08, 0.06, 0.18)
	_chi_bar_bg.custom_minimum_size = Vector2(160, 10)
	stats.add_child(_chi_bar_bg)

	_chi_bar_fill = ColorRect.new()
	_chi_bar_fill.color = Color(0.55, 0.22, 0.95)
	_chi_bar_fill.size = Vector2(160, 10)
	_chi_bar_bg.add_child(_chi_bar_fill)

# ── Mapa del dungeon ──────────────────────────────────────────────────────────

func _on_map(data: Dictionary) -> void:
	var map_w: int    = data.get("map_width",  0)
	var map_h: int    = data.get("map_height", 0)
	var b64: String   = data.get("tiles",      "")
	var tileset: String = data.get("tileset",  "walls_floor")
	var seed: int     = int(data.get("seed",   0))
	if map_w == 0 or b64.is_empty():
		return
	var raw: PackedByteArray = Marshalls.base64_to_raw(b64)

	# ── Cargar mapping del tileset activo ──────────────────────────────────
	var json_path := DUNGEON_PATH + tileset + ".json"
	var mapping   := TileLoader.load_mapping(json_path)
	if mapping.is_empty():
		push_error("Game: no se pudo cargar tileset '%s'" % json_path)
		return
	var sid := TileLoader.source_id(mapping)
	var tile_set := TileLoader.build_tileset(mapping)
	# Scale so every tile renders at TILE_SIZE px regardless of source tile_size.
	var ts_size: int = mapping.get("_meta", {}).get("tile_size", 16)
	var ts_scale := float(TILE_SIZE) / float(ts_size)

	# ── Capa de suelo ──────────────────────────────────────────────────────
	var floor_layer := TileMapLayer.new()
	floor_layer.tile_set = tile_set
	floor_layer.scale    = Vector2(ts_scale, ts_scale)
	dungeon_root.add_child(floor_layer)

	# ── Capa de muros (encima del suelo) ───────────────────────────────────
	var wall_layer := TileMapLayer.new()
	wall_layer.tile_set = tile_set
	wall_layer.scale    = Vector2(ts_scale, ts_scale)
	dungeon_root.add_child(wall_layer)

	# ── Pintar tiles ───────────────────────────────────────────────────────
	for ty in range(map_h):
		for tx in range(map_w):
			var v: int = raw[ty * map_w + tx]
			if v == 0:
				continue
			# Suelo con variante determinista según posición + seed del dungeon
			var floor_coords := TileLoader.variant(mapping, FLOOR_VARIANTS, tx, ty, seed)
			floor_layer.set_cell(Vector2i(tx, ty), sid, floor_coords)

			if v == 1:  # muro → bitmask de 4 vecinos
				var bm: int = 0
				if ty > 0         and raw[(ty-1)*map_w + tx]  == 1: bm |= 1  # N
				if tx < map_w - 1 and raw[ty*map_w + (tx+1)]  == 1: bm |= 2  # E
				if ty < map_h - 1 and raw[(ty+1)*map_w + tx]  == 1: bm |= 4  # S
				if tx > 0         and raw[ty*map_w + (tx-1)]  == 1: bm |= 8  # W
				var wall_coords := TileLoader.coords(mapping, WALL_BITMASK_NAMES[bm])
				wall_layer.set_cell(Vector2i(tx, ty), sid, wall_coords)

# ── Snapshot ──────────────────────────────────────────────────────────────────

func _on_snapshot(entities: Array) -> void:
	var seen := {}
	for e in entities:
		var id: int = int(e.get("id", 0))
		seen[id] = true
		if not _entities.has(id):
			_entities[id] = _create_entity_node(id, int(e.get("kind", 0)))
		_update_entity_node(_entities[id], e)

	for id in _entities.keys():
		if not seen.has(id):
			_entities[id].queue_free()
			_entities.erase(id)

	if _my_id != -1 and _entities.has(_my_id):
		var me := _entities[_my_id] as Node2D
		if me:
			camera.global_position = me.global_position

func _on_connected(entity_id: int, player_name: String) -> void:
	_my_id = entity_id
	_add_chat_line(tr("CHAT_SYSTEM"), tr("CHAT_WELCOME") % [player_name, entity_id])

func _on_disconnected() -> void:
	_add_chat_line(tr("CHAT_SYSTEM"), tr("CHAT_DISCONNECTED"))

func _create_entity_node(id: int, kind: int) -> Node2D:
	var node := Node2D.new()
	node.set_meta("eid",  id)
	node.set_meta("kind", kind)
	entity_root.add_child(node)

	match kind:
		0, 2:  # Jugador y Mob — sprite animado
			var spr := AnimatedSprite2D.new()
			spr.name            = "Spr"
			spr.sprite_frames   = _frames_player if kind == 0 else _frames_mob
			spr.scale           = SPR_SCALE
			spr.play("idle")
			node.add_child(spr)

	# Draw callback para todos: HP bar y nombre en players/mobs, cuerpo completo en proyectiles/items
	node.draw.connect(_draw_entity.bind(node))
	return node

func _update_entity_node(node: Node2D, e: Dictionary) -> void:
	var kind:   int    = int(e.get("kind",    0))
	var el_idx: int    = int(e.get("element", 0))
	var hp:     int    = int(e.get("hp",      0))
	var max_hp: int    = int(e.get("max_hp",  100))
	var name_s: String = e.get("name", "")
	var is_me:  bool   = (int(e.get("id", -1)) == _my_id)

	var new_pos := Vector2(float(e.get("x", 0)), float(e.get("y", 0)))
	var prev_pos := new_pos
	if node.has_meta("prev_pos"):
		prev_pos = node.get_meta("prev_pos")

	node.global_position = new_pos
	node.set_meta("prev_pos", new_pos)
	node.set_meta("el",     el_idx)
	node.set_meta("hp",     hp)
	node.set_meta("max_hp", max_hp)
	node.set_meta("ename",  name_s)
	node.set_meta("is_me",  is_me)
	node.set_meta("lv",     int(e.get("level", 1)))
	node.queue_redraw()

	# Actualiza sprite y animación para jugadores y mobs
	if kind == 0 or kind == 2:
		var spr := node.get_node_or_null("Spr") as AnimatedSprite2D
		if spr:
			var delta := new_pos - prev_pos
			if delta.length_squared() > 0.5:
				spr.play("walk")
				if absf(delta.x) > absf(delta.y):
					spr.flip_h = delta.x < 0.0
			else:
				spr.play("idle")
			var col: Color = EL_COLOR[el_idx] if el_idx < EL_COLOR.size() else Color.WHITE
			spr.modulate = col

	if is_me:
		hp_label.text     = tr("HUD_HP") % [hp, max_hp]
		el_label.text     = _el_name(el_idx)
		el_label.modulate = EL_COLOR[el_idx] if el_idx < EL_COLOR.size() else Color.WHITE
		var lv: int      = int(e.get("level",   1))
		var xp: int      = int(e.get("xp",      0))
		var xp_next: int = int(e.get("xp_next", 50))
		lv_label.text = tr("HUD_LEVEL") % lv
		var xp_ratio := clampf(float(xp) / float(max(xp_next, 1)), 0.0, 1.0)
		xp_bar_fill.size.x = xp_bar_bg.size.x * xp_ratio
		# Barra HP
		if _hp_bar_bg and _hp_bar_fill:
			var hp_ratio := clampf(float(hp) / float(max(max_hp, 1)), 0.0, 1.0)
			_hp_bar_fill.size.x = _hp_bar_bg.size.x * hp_ratio
			_hp_bar_fill.color = Color(0.85, 0.18, 0.18) if hp_ratio > 0.25 else Color(0.95, 0.5, 0.1)
		# Barra Chi — color varía con el elemento
		if _chi_bar_bg and _chi_bar_fill:
			var el_col: Color = EL_COLOR[el_idx] if el_idx < EL_COLOR.size() else Color(0.55, 0.22, 0.95)
			_chi_bar_fill.color = el_col
			_chi_bar_fill.size.x = _chi_bar_bg.size.x * _el_power

# ── Draw (HP bar, nombre, proyectiles, items) ─────────────────────────────────

func _draw_entity(node: Node2D) -> void:
	var kind:   int    = node.get_meta("kind",   0)
	var el:     int    = node.get_meta("el",      0)
	var hp:     int    = node.get_meta("hp",      0)
	var max_hp: int    = node.get_meta("max_hp",  100)
	var ename:  String = node.get_meta("ename",   "")
	var is_me:  bool   = node.get_meta("is_me",   false)
	var color: Color = EL_COLOR[el] if el < EL_COLOR.size() else Color.WHITE

	match kind:
		0:  # Jugador — "Name Lvl 99" en una línea, HP bar debajo
			var font   := ThemeDB.fallback_font
			var lv: int = node.get_meta("lv", 1)
			var name_line := "%s Lvl %d" % [ename, lv]
			# <Avatar> encima si el elemento es None (domina todos)
			if el == 0:
				var aw := font.get_string_size("<Avatar>", HORIZONTAL_ALIGNMENT_LEFT, -1, 11).x
				node.draw_string(font, Vector2(-aw * 0.5, -PLAYER_R - 34), "<Avatar>",
						HORIZONTAL_ALIGNMENT_LEFT, -1, 11, Color(1.0, 0.85, 0.2))
			var nw := font.get_string_size(name_line, HORIZONTAL_ALIGNMENT_LEFT, -1, 12).x
			node.draw_string(font, Vector2(-nw * 0.5, -PLAYER_R - 21), name_line,
					HORIZONTAL_ALIGNMENT_LEFT, -1, 12,
					Color.WHITE if is_me else color)
			_draw_hp_bar(node, PLAYER_R + 12.0, hp, max_hp)

		1:  # Proyectil — cuerpo procedural
			var R := 5.0 if el != 3 else 8.0
			node.draw_circle(Vector2.ZERO, R + 5, Color(color, 0.3))
			node.draw_circle(Vector2.ZERO, R, color)

		2:  # Mob — HP bar y nivel
			var mob_lv: int = node.get_meta("lv", 1)
			var font := ThemeDB.fallback_font
			var mob_text := "Lvl %d" % mob_lv
			var mw := font.get_string_size(mob_text, HORIZONTAL_ALIGNMENT_LEFT, -1, 12).x
			node.draw_string(font, Vector2(-mw * 0.5, -PLAYER_R - 22),
					mob_text, HORIZONTAL_ALIGNMENT_LEFT, -1, 12, color)
			_draw_hp_bar(node, PLAYER_R + 8.0, hp, max_hp)

		3:  # Item — orbe verde animado
			var t  := Time.get_ticks_msec() / 600.0
			var bob := sin(t + float(node.get_meta("eid", 0))) * 3.0
			var pos := Vector2(0.0, bob)
			node.draw_circle(pos, 12, Color(0.086, 0.396, 0.204, 0.5))
			node.draw_circle(pos, 8,  Color(0.102, 0.396, 0.204))
			node.draw_arc(pos, 8, 0, TAU, 16, Color(0.302, 0.937, 0.502), 2.0)

func _draw_hp_bar(node: Node2D, above_r: float, hp: int, max_hp: int) -> void:
	if max_hp <= 0:
		return
	var bw  := 38.0
	var bh  := 4.0
	var by  := -above_r - 4.0
	var ratio := clampf(float(hp) / float(max_hp), 0.0, 1.0)
	var bar_color: Color = Color(0.290, 0.867, 0.502) if ratio > 0.5 else \
	                       Color(0.980, 0.800, 0.082) if ratio > 0.25 else \
	                       Color(0.973, 0.529, 0.529)
	node.draw_rect(Rect2(-bw / 2.0, by, bw, bh),           Color(0.118, 0.180, 0.235))
	node.draw_rect(Rect2(-bw / 2.0, by, bw * ratio, bh),   bar_color)
	node.draw_rect(Rect2(-bw / 2.0, by, bw, bh),           Color(0.216, 0.255, 0.318), false, 1.0)

# ── Input ──────────────────────────────────────────────────────────────────────

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

	var aim := Vector2.DOWN
	if _my_id != -1 and _entities.has(_my_id):
		var me := _entities[_my_id] as Node2D
		if me:
			var mouse_world := get_viewport().get_canvas_transform().affine_inverse() \
					* get_viewport().get_mouse_position()
			aim = (mouse_world - me.global_position).normalized()

	Network.send_input(mx, my, _firing, aim.x, aim.y)
	tick_label.text = "Tick: ?"

func _input(event: InputEvent) -> void:
	if event.is_action_pressed("chat") and not chat_input.has_focus():
		chat_input.grab_focus()
		get_viewport().set_input_as_handled()

# ── Chat ───────────────────────────────────────────────────────────────────────

func _send_chat(text: String) -> void:
	text = text.strip_edges()
	if not text.is_empty():
		Network.send_chat(text)
	chat_input.text = ""
	chat_input.release_focus()

func _on_chat(_entity_id: int, name_s: String, _text: String) -> void:
	_add_chat_line(name_s, _text)

func _add_chat_line(speaker: String, text: String) -> void:
	chat_log.append_text("[color=#93c5fd]%s:[/color] %s\n" % [speaker, text])
	await get_tree().process_frame
	chat_log.scroll_to_line(chat_log.get_line_count() - 1)
