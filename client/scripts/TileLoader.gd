## TileLoader — carga y construye TileSets desde archivos JSON.
##
## Formato JSON esperado:
##   { "texture_file": "foo.png", "tile_size": 16, "source_id": 0,
##     "tiles": { "nombre": [col, row], ... } }
##
## Uso:
##   var mapping = TileLoader.load_mapping("res://assets/dungeon/walls_floor.json")
##   var tile_set = TileLoader.build_tileset(mapping)
##   var coords   = TileLoader.coords(mapping, "pared_front")  # → Vector2i(1,2)

## Carga un JSON de tileset y devuelve el mapping completo.
## Retorna {} si el archivo no existe o hay error de parseo.
static func load_mapping(json_path: String) -> Dictionary:
	if not FileAccess.file_exists(json_path):
		push_error("TileLoader: no se encontró '%s'" % json_path)
		return {}
	var file := FileAccess.open(json_path, FileAccess.READ)
	var json := JSON.new()
	if json.parse(file.get_as_text()) != OK:
		push_error("TileLoader: error parseando '%s'" % json_path)
		return {}
	file.close()

	var data: Dictionary = json.get_data()
	var raw_tiles: Dictionary = data.get("tiles", {})
	var source_id: int = data.get("source_id", 0)
	var tile_size: int = data.get("tile_size", 16)
	var tex_file: String = data.get("texture_file", "")

	# Resuelve la ruta de la textura relativa al JSON
	var base_dir := json_path.get_base_dir()
	var tex_path := base_dir.path_join(tex_file)

	var mapping := {
		"_meta": {
			"texture_path": tex_path,
			"source_id":    source_id,
			"tile_size":    tile_size,
		}
	}
	for key in raw_tiles:
		var coords = raw_tiles[key]
		mapping[key] = {
			"source_id": source_id,
			"coords":    Vector2i(int(coords[0]), int(coords[1])),
		}
	return mapping

## Construye un TileSet con un TileSetAtlasSource desde el mapping.
## Solo registra los tiles que se usan — no requiere crear todos los del atlas.
static func build_tileset(mapping: Dictionary) -> TileSet:
	var meta: Dictionary = mapping.get("_meta", {})
	var tex_path: String = meta.get("texture_path", "")
	var tile_size: int   = meta.get("tile_size", 16)
	var source_id: int   = meta.get("source_id", 0)

	var tex := load(tex_path) as Texture2D
	if tex == null:
		push_error("TileLoader: no se pudo cargar textura '%s'" % tex_path)
		return TileSet.new()

	var atlas := TileSetAtlasSource.new()
	atlas.texture = tex
	atlas.texture_region_size = Vector2i(tile_size, tile_size)

	# Registra solo los tiles nombrados (evita crear tiles vacíos)
	var seen := {}
	for key in mapping:
		if key == "_meta":
			continue
		var coords: Vector2i = mapping[key]["coords"]
		var k := str(coords)
		if not seen.has(k):
			atlas.create_tile(coords)
			seen[k] = true

	var ts := TileSet.new()
	ts.tile_size = Vector2i(tile_size, tile_size)
	ts.add_source(atlas, source_id)
	return ts

## Devuelve las coordenadas de atlas de un tile por nombre.
## Fallback al primer tile disponible si el nombre no existe.
static func coords(mapping: Dictionary, name: String) -> Vector2i:
	if mapping.has(name):
		return mapping[name]["coords"]
	push_warning("TileLoader: tile '%s' no encontrado en mapping" % name)
	# Fallback: primer tile del mapping
	for key in mapping:
		if key != "_meta":
			return mapping[key]["coords"]
	return Vector2i.ZERO

## source_id del mapping (necesario al pintar celdas del TileMapLayer).
static func source_id(mapping: Dictionary) -> int:
	return mapping.get("_meta", {}).get("source_id", 0)

## Elige entre varios nombres de variante usando (x, y, seed) para
## variación determinista — mismo seed → mismo resultado, seed diferente → look diferente.
static func variant(mapping: Dictionary, names: Array, tx: int, ty: int, seed: int) -> Vector2i:
	if names.is_empty():
		return Vector2i.ZERO
	var idx := absi((tx * 1619 + ty * 31337 + seed * 6271)) % names.size()
	return coords(mapping, names[idx])
