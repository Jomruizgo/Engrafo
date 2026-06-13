# engrafo

> **Estado:** experimental. La arquitectura está fundamentada, los casos de uso son reales, pero el beneficio en tokens y en reducción de contexto no ha sido benchmarkeado sistemáticamente. Úsalo, mide, y reporta.

Servidor MCP en Go que combina grafo de dependencias bi-temporal con memoria episódica anclada, diseñado para reducir el consumo de tokens y eliminar el miedo a limpiar la ventana de contexto.

---

## Por qué existe

Los agentes de coding ya pueden consultar la estructura del código. Herramientas como Graphify, LSP, o búsqueda semántica responden "¿qué depende de X?". engram guarda observaciones sobre lo que ocurrió en sesiones anteriores. Ambas cosas existen por separado.

El problema es la desconexión. Cuando el agente recuerda "este módulo tuvo un bug por su dependencia en (legacyValidator)", esa memoria vive en lenguaje natural sin vínculo al nodo real del grafo. Si (legacyValidator) desaparece del código, la observación queda huérfana. No hay forma de saber si lo que se aprendió sigue siendo relevante.

engrafo nació para resolver eso: **anclar observaciones episódicas a nodos del grafo con historia bi-temporal**. El resultado es que cuando una dependencia cambia, las memorias asociadas a ese cambio son recuperables en contexto.

## Qué combina

| Componente | Fuente de inspiración | Lo que engrafo añade |
|---|---|---|
| Grafo de dependencias | Graphify, code-review-graph | Modelo bi-temporal: aristas con (valid_from_commit) y (valid_until_commit) |
| Memoria episódica | engram | Anclaje de observaciones a nodos específicos del grafo |
| Detección de dead code | Análisis estático clásico | Detecta "abandonado" (tuvo referencias, las perdió) gracias al historial bi-temporal |

## El problema del contexto que engrafo intenta resolver

La ventana de contexto de un agente es memoria de trabajo: rápida, limitada, volátil. Cuando se llena o se compacta, se pierde información sobre decisiones tomadas, enfoques descartados, bugs resueltos. La reacción habitual es no limpiarla, acumulando tokens que encarecen cada llamada.

El cerebro humano no funciona así. No replays todo lo que ocurrió desde que naciste para retomar una tarea. Tienes:
- **Memoria episódica** (qué pasó, cuándo, en qué contexto) → engram
- **Memoria semántica/estructural** (cómo funciona algo, qué depende de qué) → engrafo
- **Memoria de trabajo** (lo que estás procesando ahora) → la ventana de contexto

La combinación de las dos primeras permite vaciar la tercera sin perder el hilo. Al inicio de una sesión, los hooks inyectan: contexto estructural del código (cg_context) + observaciones episódicas relevantes de sesiones anteriores (mem_context). El agente retoma el trabajo con el mismo mapa cognitivo, sin necesidad de haber acumulado miles de tokens de conversación previa.

En teoría esto debería funcionar porque la información que se pierde al limpiar el contexto (decisiones, errores, convenciones del proyecto) es exactamente lo que engram y engrafo persisten de forma estructurada. La hipótesis no está benchmarkeada; el diseño sí está fundamentado.

## El grafo como alternativa a grep para reducir tokens

Sin un grafo de dependencias, cuando el agente necesita entender la estructura del código hace búsquedas: grep sobre el árbol de archivos, lecturas de archivos para extraer imports y llamadas, y así recursivamente hasta tener el mapa. Cada archivo leído entra completo a la ventana de contexto aunque el agente solo necesite saber "quién depende de quién".

Con engrafo, esa estructura está pre-indexada en SQLite. El agente emite una sola consulta al servidor MCP (por ejemplo, "qué archivos se ven afectados si cambio este módulo") y recibe únicamente la lista de nodos y aristas relevantes, sin cargar el contenido de ningún archivo. La diferencia en tokens es la que hay entre leer una tabla de contenidos y leer el libro completo.

La ventaja es mayor cuanto más grande es el proyecto. En proyectos pequeños grep es suficiente. A partir de cierta escala, el grafo evita que el agente tenga que re-explorar la misma estructura en cada sesión o cada vez que necesita orientarse tras una compactación de contexto.

---

## Instalación

El binario se instala **una vez por máquina** (instalación global). El índice del grafo y los hooks se configuran **por proyecto**: hay que estar ubicado en la carpeta del proyecto al ejecutar (engrafo init) y (engrafo hooks install).

```
máquina:   go install / brew / scoop  →  binario disponible en PATH
proyecto:  cd /tu/proyecto && engrafo init  →  crea .engrafo/graph.db
proyecto:  engrafo hooks install  →  configura .claude/settings.json
```

### go install (recomendado)

Requiere Go 1.22+. CGO no requerido.

```sh
go install github.com/Jomruizgo/Engrafo/cmd/engrafo@latest
```

Verifica:

```sh
engrafo doctor
```

### Homebrew (macOS / Linux)

```sh
brew install jomruizgo/tap/engrafo
```

### Scoop (Windows)

```sh
scoop install engrafo
```

### Binario precompilado

Descarga el binario para tu plataforma desde [Releases](https://github.com/Jomruizgo/Engrafo/releases) y ponlo en tu `PATH`.

| Plataforma       | Archivo                          |
|-----------------|----------------------------------|
| Linux amd64     | `engrafo_linux_amd64.tar.gz`     |
| Linux arm64     | `engrafo_linux_arm64.tar.gz`     |
| macOS amd64     | `engrafo_darwin_amd64.tar.gz`    |
| macOS arm64     | `engrafo_darwin_arm64.tar.gz`    |
| Windows amd64   | `engrafo_windows_amd64.zip`      |

---

## Quickstart

```sh
# 1. Indexar el repositorio actual
cd /tu/proyecto
engrafo init

# 2. Instalar hooks + engram (auto-detecta e instala engram si no está)
engrafo hooks install

# 3. Verificar instalación completa
engrafo doctor
```

`hooks install` configura automáticamente los dos MCP servers (`engrafo` + `engram`) en el agente y registra los hooks de sesión.

Para historia bi-temporal completa (mejor detección de dead code):

```sh
engrafo init --from-git 50   # replaya los últimos 50 commits
```

---

## Integración con engram

engrafo se integra con [engram](https://github.com/Gentleman-Programming/engram), el sistema de memoria episódica del ecosistema Gentle-AI. La integración es **opcional pero recomendada**: sin ella las herramientas estructurales funcionan completas, pero (cg_anchor) y el contenido de observaciones en (cg_history) quedan inactivos.

**engram se instala automáticamente** al ejecutar (engrafo hooks install). Si ya tienes Gentle-AI configurado, engram ya está instalado.

### Compatibilidad de versiones

| Situación | Comportamiento de `hooks install` |
|---|---|
| engram no instalado | Instala automáticamente la versión testeada |
| engram en versión antigua | Actualiza a la versión testeada |
| engram en versión testeada | OK, no hace nada |
| engram en versión más nueva | Avisa, no hace downgrade (responsabilidad del usuario) |

La versión de engram testeada con esta release está fijada en (internal/version.EngramCompatible).

### Sin engram

| Herramienta | Sin engram |
|---|---|
| `cg_context`, `cg_node`, `cg_dependents`, `cg_dependencies`, `cg_impact`, `cg_search`, `cg_deadcode` | Funcionan completamente |
| `cg_anchor` | Acepta llamadas pero los IDs no se resuelven |
| `cg_history` | Devuelve el timeline de aristas; los `anchored_observations` son IDs vacíos |

---

## Integración con agentes

### Claude Code

Agrega esto a `.claude/settings.json` de tu proyecto (o ejecuta `engrafo hooks install` para hacerlo automáticamente):

```json
{
  "mcpServers": {
    "engrafo": {
      "command": "engrafo",
      "args": ["serve"],
      "env": {}
    }
  }
}
```

Los hooks se instalan en `hooks.SessionStart`, `hooks.PreToolUse` y `hooks.PostToolUse` para inyectar contexto estructural automáticamente.

### GitHub Copilot

```sh
engrafo hooks install --agent copilot
```

Crea los hooks en `.github/hooks/`.

### OpenCode

```sh
engrafo hooks install --agent opencode
```

Configura `.opencode/settings.json`.

---

## Comandos CLI

```
engrafo [--db <ruta>] <comando> [args]
```

| Comando                        | Descripción                                                                 |
|-------------------------------|-----------------------------------------------------------------------------|
| `init [root]`                 | Indexa el repositorio desde cero. Crea `.engrafo/graph.db`.                |
| `init --from-git N [root]`    | Replaya los últimos N commits para construir historia bi-temporal.          |
| `update`                      | Actualización incremental: solo archivos cambiados desde el último commit.  |
| `serve`                       | Inicia el servidor MCP en stdio. Usado por la configuración del agente.     |
| `ui [--port N]`               | Abre el navegador de grafo en `http://localhost:8080` (defecto: 8080).      |
| `deadcode [--json] [--threshold-days N]` | Lista candidatos a dead code (huérfanos y abandonados).        |
| `status`                      | Muestra estadísticas del índice: nodos, aristas, último commit.             |
| `query <symbol>`              | Consulta un símbolo directamente desde la CLI.                              |
| `hooks install`               | Instala hooks de Claude Code en `.claude/settings.json`.                   |
| `hooks uninstall`             | Elimina los hooks instalados.                                               |
| `doctor`                      | Verifica la instalación: BD, schema, soporte FTS5.                          |

**Flag global:**

`--db <ruta>` — ruta explícita al archivo `graph.db`. Por defecto se detecta automáticamente desde el directorio git raíz como `.engrafo/graph.db`.

---

## Herramientas MCP

El servidor expone 9 herramientas al agente:

### `cg_context`
Resumen del proyecto: total de nodos, lenguajes, símbolos más referenciados, conteo por tipo. Primera llamada recomendada al inicio de sesión.

```json
{ }
```

### `cg_node`
Detalle completo de un símbolo: aristas activas (depends_on, used_by), aristas históricas invalidadas, observaciones de engram ancladas.

```json
{
  "symbol": "UserRepository",
  "kind": "class",               // opcional: function, class, method, interface, package
  "include_invalidated": true    // opcional: incluye aristas históricas
}
```

### `cg_dependents`
¿Qué se rompe si cambio este archivo? Nodos con aristas entrantes hacia el objetivo.

```json
{ "file_path": "internal/user/repo.go" }
```

### `cg_dependencies`
¿De qué depende este archivo? Aristas salientes del objetivo.

```json
{ "file_path": "internal/user/repo.go" }
```

### `cg_impact`
Radio de blast transitivo: todos los archivos afectados por un cambio, hasta N saltos de profundidad.

```json
{ "file_path": "internal/user/repo.go", "depth": 3 }
```

### `cg_search`
Búsqueda FTS5 sobre nombres y firmas de símbolos. Soporta sintaxis de match SQLite FTS5.

```json
{ "query": "UserRepo*", "limit": 10 }
```

### `cg_deadcode`
Detecta código muerto:
- **Huérfanos:** nodos que nunca tuvieron ninguna arista entrante.
- **Abandonados:** nodos que tenían aristas entrantes y las perdieron todas. El modelo bi-temporal hace esta detección altamente precisa.

```json
{ "threshold_days": 30 }   // opcional: solo inactivos por más de N días
```

### `cg_history`
Timeline cronológico de aristas para un símbolo: cuándo aparecieron y desaparecieron dependencias, con qué commit.

```json
{ "symbol": "processOrder", "kind": "function" }
```

Respuesta:
```json
{
  "symbol": "processOrder",
  "kind": "function",
  "file_path": "internal/order/service.go",
  "language": "go",
  "timeline": [
    { "commit": "abc123", "event_type": "appeared", "target_symbol": "validateOrder", "edge_kind": "calls" },
    { "commit": "def456", "event_type": "disappeared", "target_symbol": "legacyCheck", "edge_kind": "calls" }
  ],
  "anchored_observations": ["obs-uuid-001"]
}
```

### `cg_anchor`
Vincula una observación de engram a uno o más nodos del grafo por símbolo.

```json
{
  "engram_obs_id": "obs-uuid-001",
  "symbols": ["UserRepository", "processOrder"]
}
```

---

## Navegador de grafo (UI)

```sh
engrafo ui
# → http://localhost:8080

engrafo ui --port 3000
# → http://localhost:3000
```

Interfaz local read-only que muestra:
- Lista de símbolos con búsqueda FTS5
- Detalle de nodo: aristas activas, aristas históricas invalidadas, observaciones de engram ancladas
- Tab de dead code: huérfanos y abandonados

No requiere internet. No instala dependencias adicionales (el HTML está embebido en el binario).

---

## Arquitectura

```
engrafo/
├── cmd/engrafo/          # CLI: init, update, serve, ui, deadcode, hooks, doctor
├── internal/
│   ├── graph/            # Store SQLite + Builder (bi-temporal) + Querier
│   ├── mcp/              # Servidor MCP stdio, 9 herramientas
│   ├── parser/           # Parser multi-lenguaje con tree-sitter
│   │   └── extractors/   # Go, TypeScript, Python (requieren CGO)
│   └── ui/               # Servidor HTTP + SPA embebida (sin CGO)
├── schema/               # plugin.json + tests de integridad
└── plugin.json           # Manifiesto MCP para registros de agentes
```

### Modelo de datos

- **Nodos:** símbolos (funciones, clases, interfaces, paquetes) y archivos.
- **Aristas:** dirigidas, con (valid_from_commit) para la aparición y (valid_until_commit) para la desaparición. Nunca se eliminan — se invalidan.
- **Anchors:** tabla (engram_anchors) vincula (engram_obs_id) → (node_id).

### CGO

| Funcionalidad         | CGO requerido |
|----------------------|---------------|
| Parser tree-sitter   | Sí            |
| SQLite (modernc)     | No            |
| MCP server           | No            |
| UI server            | No            |

El binario funciona sin CGO. Sin CGO, `engrafo init` requiere que el código fuente ya esté parseado o se use un extractor alternativo. La distribución oficial incluye CGO habilitado.

---

## Desarrollo

```sh
git clone https://github.com/Jomruizgo/Engrafo
cd Engrafo

# Tests (sin CGO):
go test ./... -count=1

# Tests con CGO (requiere GCC compatible):
CGO_ENABLED=1 go test ./... -count=1 -tags cgo

# Build:
go build ./cmd/engrafo
```

**Nota para Windows:** MSYS2 con GCC 16+ rompe CGO con Go 1.25.x. Usar `CGO_ENABLED=0` para tests y desarrollo sin parser tree-sitter.

---

## Licencia

MIT — ver [LICENSE](LICENSE).
