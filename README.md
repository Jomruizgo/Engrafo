# engrafo

Servidor MCP en Go que construye un grafo estructural de tu código y lo expone a agentes de IA (Claude Code, GitHub Copilot, OpenCode). Sin runtime externo, sin API keys.

**Por qué existe:** Los agentes de coding tienen buena memoria episódica (engram, etc.) pero no saben la estructura del código. `engrafo` responde preguntas como *"¿qué se rompe si modifico `UserRepository`?"* sin releer el código fuente.

**Diferenciador:** El grafo es **bi-temporal** — las aristas nunca se borran, se invalidan con el commit en que dejaron de existir. Esto permite detectar código abandonado con alta precisión y anclar observaciones de engram a nodos del grafo para que el agente no repita errores descartados.

---

## Instalación

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

# 2. Instalar hooks en Claude Code
engrafo hooks install

# 3. Iniciar el servidor MCP
engrafo serve
```

Para historia bi-temporal completa (mejor detección de dead code):

```sh
engrafo init --from-git 50   # replaya los últimos 50 commits
```

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
- **Aristas:** dirigidas, con `valid_from_commit` (aparición) y `valid_until_commit` (desaparición). Nunca se eliminan — se invalidan.
- **Anchors:** tabla `engram_anchors` vincula `engram_obs_id` → `node_id`.

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
