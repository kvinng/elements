## Lobby.gd — Pantalla de selección de nombre y elemento.
extends Control

const ELEMENTS := {
	"fire":  {"label": "FUEGO",      "color": Color(0.976, 0.451, 0.086), "desc": "100HP · 210spd · Vence a Aire (+50%)"},
	"water": {"label": "AGUA",       "color": Color(0.376, 0.647, 0.980), "desc": "130HP · 170spd · Vence a Fuego (+50%)"},
	"earth": {"label": "TIERRA",     "color": Color(0.639, 0.898, 0.208), "desc": "160HP · 140spd · Vence a Agua (+50%)"},
	"air":   {"label": "AIRE",       "color": Color(0.886, 0.910, 0.941), "desc": "70HP · 270spd · Triple disparo · Vence a Tierra (+50%)"},
	"none":  {"label": "NO-MAESTRO", "color": Color(0.753, 0.518, 0.988), "desc": "100HP · 200spd · Sin ventaja elemental"},
}

var _selected_element := "fire"

@onready var name_input:   LineEdit = $VBox/NameInput
@onready var detail_label: Label    = $VBox/DetailLabel
@onready var enter_btn:    Button   = $VBox/EnterBtn
@onready var el_grid:      GridContainer = $VBox/ElementGrid

func _ready() -> void:
	_build_element_buttons()
	enter_btn.pressed.connect(_on_enter)
	name_input.text_submitted.connect(func(_t): _on_enter())
	_update_detail()

func _build_element_buttons() -> void:
	for key in ELEMENTS:
		var info = ELEMENTS[key]
		var btn := Button.new()
		btn.text = info["label"]
		btn.custom_minimum_size = Vector2(140, 60)
		btn.pressed.connect(_select_element.bind(key, btn))
		btn.set_meta("el_key", key)
		el_grid.add_child(btn)

func _select_element(key: String, btn: Button) -> void:
	_selected_element = key
	# Reset all button modulate
	for child in el_grid.get_children():
		child.modulate = Color.WHITE
	btn.modulate = ELEMENTS[key]["color"]
	_update_detail()

func _update_detail() -> void:
	var info = ELEMENTS[_selected_element]
	detail_label.text = info["desc"]
	detail_label.modulate = info["color"]

func _on_enter() -> void:
	var player_name := name_input.text.strip_edges()
	if player_name.is_empty():
		player_name = "Aventurero"
	Network.connect_to_server(player_name, _selected_element)
	get_tree().change_scene_to_file("res://scenes/Game.tscn")
