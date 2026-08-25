## Lobby.gd — Login / Registro antes de entrar al juego.
extends Control

# Datos por elemento: color para UI + clave de traducción para nombre y descripción.
# Añadir idiomas solo requiere editar los .po, no este código.
const ELEMENTS := {
	"fire":  {"key": "EL_FIRE",  "desc_key": "EL_FIRE_DESC",  "color": Color(0.976, 0.451, 0.086)},
	"water": {"key": "EL_WATER", "desc_key": "EL_WATER_DESC", "color": Color(0.376, 0.647, 0.980)},
	"earth": {"key": "EL_EARTH", "desc_key": "EL_EARTH_DESC", "color": Color(0.639, 0.898, 0.208)},
	"air":   {"key": "EL_AIR",   "desc_key": "EL_AIR_DESC",   "color": Color(0.886, 0.910, 0.941)},
	"none":  {"key": "EL_NONE",  "desc_key": "EL_NONE_DESC",  "color": Color(0.753, 0.518, 0.988)},
}

var _mode := "login"       # "login" | "register"
var _selected_element := "fire"
var _busy := false

@onready var name_input:   LineEdit      = $VBox/NameInput
@onready var pass_input:   LineEdit      = $VBox/PasswordInput
@onready var el_grid:      GridContainer = $VBox/ElementGrid
@onready var detail_label: Label         = $VBox/DetailLabel
@onready var enter_btn:    Button        = $VBox/EnterBtn
@onready var mode_btn:     Button        = $VBox/ModeBtn
@onready var error_label:  Label         = $VBox/ErrorLabel

func _ready() -> void:
	name_input.placeholder_text = tr("LOBBY_NAME_PLACEHOLDER")
	pass_input.placeholder_text = tr("LOBBY_PASS_PLACEHOLDER")
	_build_element_buttons()
	enter_btn.pressed.connect(_on_enter)
	mode_btn.pressed.connect(_toggle_mode)
	name_input.text_submitted.connect(func(_t): _on_enter())
	pass_input.text_submitted.connect(func(_t): _on_enter())
	Network.auth_success.connect(_on_auth_success)
	Network.auth_failed.connect(_on_auth_failed)
	_apply_mode()

func _build_element_buttons() -> void:
	for key in ELEMENTS:
		var info = ELEMENTS[key]
		var btn := Button.new()
		btn.text = tr(info["key"])
		btn.custom_minimum_size = Vector2(120, 54)
		btn.pressed.connect(_select_element.bind(key, btn))
		btn.set_meta("el_key", key)
		el_grid.add_child(btn)

func _select_element(key: String, btn: Button) -> void:
	_selected_element = key
	for child in el_grid.get_children():
		child.modulate = Color.WHITE
	btn.modulate = ELEMENTS[key]["color"]
	detail_label.text = tr(ELEMENTS[key]["desc_key"])
	detail_label.modulate = ELEMENTS[key]["color"]

func _toggle_mode() -> void:
	_mode = "register" if _mode == "login" else "login"
	_apply_mode()

func _apply_mode() -> void:
	var is_register := _mode == "register"
	el_grid.visible      = is_register
	detail_label.visible = is_register
	if is_register:
		detail_label.text = tr(ELEMENTS[_selected_element]["desc_key"])
	enter_btn.text = tr("LOBBY_BTN_REGISTER") if is_register else tr("LOBBY_BTN_LOGIN")
	mode_btn.text  = tr("LOBBY_TOGGLE_TO_LOGIN") if is_register else tr("LOBBY_TOGGLE_TO_REGISTER")
	error_label.visible = false
	error_label.text = ""

func _on_enter() -> void:
	if _busy:
		return
	var player_name := name_input.text.strip_edges()
	var password    := pass_input.text
	if player_name.is_empty():
		_show_error(tr("LOBBY_ERR_NAME_EMPTY"))
		return
	if password.is_empty():
		_show_error(tr("LOBBY_ERR_PASS_EMPTY"))
		return
	_set_busy(true)

	if _mode == "login":
		Network.login(player_name, password)
	else:
		Network.register(player_name, password, _selected_element)

func _set_busy(busy: bool) -> void:
	_busy = busy
	enter_btn.disabled = busy
	mode_btn.disabled  = busy
	name_input.editable = not busy
	pass_input.editable = not busy
	enter_btn.text = tr("LOBBY_CONNECTING") if busy else (
		tr("LOBBY_BTN_REGISTER") if _mode == "register" else tr("LOBBY_BTN_LOGIN"))
	if busy:
		error_label.visible = false

func _on_auth_success(_player_data: Dictionary) -> void:
	Network.connect_ws()
	get_tree().change_scene_to_file("res://scenes/Game.tscn")

func _on_auth_failed(error: String) -> void:
	_set_busy(false)
	_show_error(error)

func _show_error(msg: String) -> void:
	error_label.text = msg
	error_label.visible = true
