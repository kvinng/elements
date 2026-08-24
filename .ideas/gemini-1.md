# Game Design Document: Project Elementals (GDD)

**Visión General:** MMORPG / Action RPG 2D de perspectiva cenital (*top-down*) con combate dinámico de proyectiles, áreas de efecto (AoE) y físicas de impacto. Destaca por su sistema de progresión de cuatro maestrías elementales, un bucle de juego basado en el prestigio, *loot* procedural y un sistema de diplomacia/PvP democrático liderado por jugadores de máximo nivel.

---

## 1. Arquitectura del Sistema de Progresión

### 1.1. Bucle de Juego Principal (Gameplay Loop)

```
[Selección de Elemento Inicial] ➔ [Leveling 1-100] ➔ [Misión de Ascensión]
                                                             │
[PvP de Facciones / Endgame] ◄── [Maestría x4 (Nivel 400)] ◄─┘ (Reset / Prestigio)

```

1. **Creación de Personaje:** El jugador elige su elemento de origen (Fuego, Agua, Tierra o Aire).
2. **Nivel Base (1-100):** Sube hasta el nivel máximo desbloqueando el árbol de habilidades de su elemento nativo.
3. **Misión de Ascensión (El Primordial / Avatar):** Al llegar a nivel 100, desbloquea una cadena de misiones en la Zona Central Neutral. Al completarla, el personaje realiza un "reset/prestigio".
4. **Reseteo y Acumulación de Maestrías:**
* El personaje vuelve a nivel 1 en su segundo elemento, pero conserva un bonificador pasivo por el elemento dominado.
* Debe subir cada uno de los 3 elementos restantes desde nivel 1 hasta nivel 100.
* **Nivel Máximo Efectivo:** 400 (100 niveles $\times$ 4 elementos).


5. **Estado Primordial:** Al dominar los 4 elementos (Nivel 400), el personaje puede equipar habilidades de cualquier elemento de forma simultánea en su barra de acceso rápido, abriendo sinergias de combate avanzadas.

---

## 2. Geografía del Mundo y Facciones

El mundo está diseñado para concentrar a la comunidad en zonas estratégicas de conflicto y fomentar el movimiento continuo entre regiones.

```
       [Reino del Agua - Polo Norte]
                     │
 [Reino del Fuego] ──┼── [Reino de la Tierra (Gran Península Central)]
 (Islas del Oeste)   │   ├── Santuarios del Viento (Cordillera)
                     │   └── Zona de Conflicto Neutral (PVP)
                     │
       [Reino del Agua - Polo Sur]

```

* **Reino de la Tierra:** El continente central dominante. Contiene las llanuras principales, pueblos mineros, la Capital y los **Santuarios del Viento** integrados en sus altas cordilleras orientales.
* **Zona de Conflicto Central (PvP):** Una gran región neutral dentro del continente de la Tierra donde todas las naciones coinciden para realizar eventos, misiones de ascensión y PvP abierto.
* **Reino del Fuego:** Un archipiélago de islas volcánicas situado al oeste del continente central.
* **Reino del Agua (Norte y Sur):** Regiones glaciares situadas en los extremos superior e inferior del mapa, sin conexión directa por tierra con el Polo Sur.

---

## 3. Sistema de Economía y Loot (Estilo Action RPG)

El equipamiento utiliza un sistema de atributos aleatorios (*affixes*) generado proceduralmente al derrotar monstruos o jefes de mazmorra.

### 3.1. Raridades de Objetos

* **Común (Blanco):** Atributos base (Defensa, Ataque).
* **Raro (Azul):** +1 a +2 prefijos aleatorios.
* **Épico (Amarillo):** +3 a +4 prefijos/sufijos (Ej. *"+8% Velocidad de Ataque con Viento"*).
* **Primordial / Legendario (Naranja):** Atributos fijos únicos enfocados en la hibridación de elementos (Ej. *"Anillo de la Convergencia: Al aplicar congelación con Agua, el siguiente golpe de Tierra causa un 30% de daño crítico adicional"*).

---

## 4. PvP y Sistema Democrático de Guerras

### 4.1. Votación de Conflicto por Censo de Primordiales / Avatares

La decisión de declarar la guerra a otra nación no recae en un solo jugador, sino en el **Censo de Jugadores Ascendidos (Avatares)** que iniciaron su aventura en la facción correspondiente.

1. **Inicio de Propuesta:** Un Avatar propone declarar la guerra a una nación rival desde el panel de facción.
2. **Censo Activo:** El servidor realiza un recuento de todos los personajes en estado Avatar/Primordial cuya facción de origen sea la nación postulante.
3. **Votación Ponderada:** Se abre una ventana de votación de 24 horas. Para que la declaración sea efectiva, se requiere superar el umbral configurado (ej. **>50% de votos afirmativos** del total del censo activo de esa facción).
4. **Fase de Hostilidad:**
* Si la votación aprueba la guerra, se activa el estado de **PvP Abierto** en la Zona Central Neutral por un periodo definido (ej. 48 horas).
* La facción victoriosa en eventos/bajas durante el conflicto obtiene modificadores de *drop rate* y bonus de experiencia en zonas fronterizas.



---

## 5. Arquitectura Técnica (Stack Recomendado)

### 5.1. Backend (Servidor Autoritativo)

* **Lenguaje:** **Go (Golang)**.
* **Protocolo:** WebSockets (para prototipado rápido) o UDP/ENet (para producción de baja latencia).
* **Frecuencia de Actualización (Tick Rate):** 20 Hz (20 simulaciones por segundo).
* **Base de Datos:** PostgreSQL para perfiles, inventario y persistencia; Redis para caché del estado de mundo y gestión de votaciones en tiempo real.

### 5.2. Frontend (Cliente)

* **Motor:** **Godot Engine 2D** (utilizando GDScript).
* **Renderizado:** Vista cenital (*top-down*) con ordenación por profundidad (*Y-sorting*).
* **Efectos Visuales (VFX):** Sistemas de partículas 2D nativos, shaders de refracción para viento/fuego y sacudida de cámara (*screen shake*) para simular peso en los impactos.

---

## 6. Hoja de Ruta de Desarrollo (Roadmap)

1. **Fase 1: Core de Red en Go**
* Creación del bucle de simulación del servidor.
* Manejo de posiciones $X, Y$, colisiones básicas y movimiento sincronizado de 2+ clientes.


2. **Fase 2: Motor de Combate 2D**
* Implementación de proyectiles autoritativos (calculados en el servidor).
* Sistema de cooldowns y cálculo de daño según el nivel del elemento.


3. **Fase 3: Cliente y Sprites en Godot**
* Integración de mapas de tiles (*tilemaps*).
* Conexión cliente-servidor mediante eventos JSON/Protobuf.


4. **Fase 4: Persistencia y Progresión**
* Modelado de la base de datos PostgreSQL.
* Implementación del sistema de reset/prestigio de los 4 elementos.


5. **Fase 5: Módulo de Diplomacia y Censo**
* Creación del sistema de votación para declaraciones de guerra basado en redis/PostgreSQL.