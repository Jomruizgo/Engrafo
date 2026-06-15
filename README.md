# Engrafo

> **Estado:** experimental. La arquitectura está fundamentada, los casos de uso son reales, pero el beneficio en tokens y en reducción de contexto no ha sido benchmarkeado sistemáticamente. Úsalo, mide, y reporta.

Servidor MCP en Go que combina grafo de dependencias bi-temporal con memoria episódica anclada, diseñado para reducir el consumo de tokens y eliminar el miedo a limpiar la ventana de contexto.

---

## Por qué existe

Los agentes de coding ya pueden consultar la estructura del código. Herramientas como Graphify, LSP, o búsqueda semántica responden "¿qué depende de X?". engram guarda observaciones sobre lo que ocurrió en sesiones anteriores. Ambas cosas existen por separado.

El problema es la desconexión. Cuando el agente recuerda que un módulo tuvo un bug causado por una dependencia específica, esa memoria vive en lenguaje natural sin vínculo al nodo real del grafo. Si esa dependencia desaparece del código, la observación queda huérfana. No hay forma de saber si lo que se aprendió sigue siendo relevante.

Engrafo nació para resolver eso: **anclar observaciones episódicas a nodos del grafo con historia bi-temporal**. El resultado es que cuando una dependencia cambia, las memorias asociadas a ese cambio son recuperables en contexto.

## Qué combina

| Componente | Fuente de inspiración | Lo que Engrafo añade |
|---|---|---|
| Grafo de dependencias | Graphify, code-review-graph | Modelo bi-temporal: aristas con (valid_from_commit) y (valid_until_commit) |
| Memoria episódica | engram | Anclaje de observaciones a nodos específicos del grafo |
| Detección de dead code | Análisis estático clásico | Detecta "abandonado" (tuvo referencias, las perdió) gracias al historial bi-temporal |

## El problema del contexto que Engrafo intenta resolver

La ventana de contexto de un agente es memoria de trabajo: rápida, limitada, volátil. Cuando se llena o se compacta, se pierde información sobre decisiones tomadas, enfoques descartados, bugs resueltos. La reacción habitual es no limpiarla, acumulando tokens que encarecen cada llamada.

El cerebro humano no funciona así. No replays todo lo que ocurrió desde que naciste para retomar una tarea. Tienes:
- **Memoria episódica** (qué pasó, cuándo, en qué contexto) → engram
- **Memoria semántica/estructural** (cómo funciona algo, qué depende de qué) → Engrafo
- **Memoria de trabajo** (lo que estás procesando ahora) → la ventana de contexto

La combinación de las dos primeras permite vaciar la tercera sin perder el hilo. Al inicio de una sesión, los hooks inyectan: contexto estructural del código (cg_context) + observaciones episódicas relevantes de sesiones anteriores (mem_context). El agente retoma el trabajo con el mismo mapa cognitivo, sin necesidad de haber acumulado miles de tokens de conversación previa.

En teoría esto debería funcionar porque la información que se pierde al limpiar el contexto (decisiones, errores, convenciones del proyecto) es exactamente lo que engram y Engrafo persisten de forma estructurada. La hipótesis no está benchmarkeada; el diseño sí está fundamentado.

## El grafo como alternativa a grep para reducir tokens

Sin un grafo de dependencias, cuando el agente necesita entender la estructura del código hace búsquedas: grep sobre el árbol de archivos, lecturas de archivos para extraer imports y llamadas, y así recursivamente hasta tener el mapa. Cada archivo leído entra completo a la ventana de contexto aunque el agente solo necesite saber "quién depende de quién".

Con Engrafo, esa estructura está pre-indexada en SQLite. El agente emite una sola consulta al servidor MCP (por ejemplo, "qué archivos se ven afectados si cambio este módulo") y recibe únicamente la lista de nodos y aristas relevantes, sin cargar el contenido de ningún archivo. La diferencia en tokens es la que hay entre leer una tabla de contenidos y leer el libro completo.

La hipótesis es que la ventaja crece con el tamaño del proyecto: en proyectos pequeños grep es probablemente suficiente, y el overhead de mantener el índice puede no justificarse. A partir de cierta escala, se espera que el grafo evite re-explorar la misma estructura en cada sesión. Cuál es ese umbral es algo que aún no se ha medido.

---

## Instalación

El binario se instala **una vez por máquina** (instalación global). El índice del grafo y los hooks se configuran **por proyecto**: hay que estar ubicado en la carpeta del proyecto al ejecutar (engrafo init) y (engrafo hooks install).

```
máquina:   go install / brew / scoop  →  binario disponible en PATH
proyecto:  cd /tu/proyecto && engrafo init  →  crea .engrafo/graph.db
proyecto:  engrafo hooks install  →  configura .claude/settings.json
```

### go install (recomendado)

Requiere Go 1.25.5+ y **CGO habilitado** (necesario para el parser tree-sitter que extrae símbolos).

> **Sin CGO el binario compila y arranca, pero `engrafo init` no extrae ningún símbolo.** Las revisiones git se registran, pero el grafo queda vacío.

```sh
CGO_ENABLED=1 go install github.com/Jomruizgo/Engrafo/v2/cmd/engrafo@latest
```

#### CGO en Windows

Requiere un compilador C compatible con Go 1.25.x. TDM-GCC 10.x está roto (produce binarios que Windows rechaza). Opciones probadas:

| Opción | GCC | Instalación |
|---|---|---|
| **MSYS2 mingw64** (recomendado) | 16.x | [msys2.org](https://www.msys2.org/) → `pacman -S mingw-w64-x86_64-gcc` |
| **MSYS2 ucrt64** | 14–16.x | `pacman -S mingw-w64-ucrt-x86_64-gcc` — variante ucrt |
| **w64devkit** | 13.x | [github.com/skeeto/w64devkit](https://github.com/skeeto/w64devkit/releases) — portable, sin instalador |

Tras instalar MSYS2, agrega `C:\msys64\mingw64\bin` al PATH de usuario y luego:

```sh
gcc --version       # debe reportar mingw64 o ucrt64, no TDM-GCC
set CGO_ENABLED=1
go install github.com/Jomruizgo/Engrafo/v2/cmd/engrafo@latest
engrafo doctor
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

## Multi-repo (workspace)

Engrafo funciona en tres modos con el mismo binario. El modo se detecta automáticamente al ejecutar cualquier comando:

| Modo | Cómo se activa | Base de datos |
|---|---|---|
| **Multi-repo** | Existe `engrafo.json` en algún directorio padre del CWD | `<workspace>/.engrafo/graph.db` |
| **Single-repo con git** | No hay manifest, pero hay `.git` en algún directorio padre | `<gitroot>/.engrafo/graph.db` |
| **Single-repo sin git** | No hay ni manifest ni `.git` | `<cwd>/.engrafo/graph.db` |

En todos los modos la capa de query es idéntica. El single-repo es un multi-repo de una sola raíz registrada automáticamente.

### Crear un workspace

```sh
cd /carpeta/que/agrupa/tus/repos   # puede ser cualquier carpeta, no tiene que ser padre de las raíces
engrafo workspace add backend-auth ./services/auth
engrafo workspace add product-front ../platform-product
engrafo workspace add e2e D:/otra/ruta/e2e
engrafo init           # indexa todas las raíces registradas
engrafo hooks install  # instala MCP + hooks una sola vez para todo el workspace
```

### Formato de `engrafo.json`

El archivo se crea y gestiona mediante `workspace add`, pero puede editarse a mano:

```json
{
  "version": 1,
  "roots": [
    { "name": "backend-auth",   "path": "./services/auth" },
    { "name": "product-front",  "path": "../platform-product" },
    { "name": "e2e",            "path": "D:/rutas/dispersas/e2e",
      "remote": "git@github.com:org/e2e.git", "branch": "main", "vcs": "git" }
  ]
}
```

Reglas: `name` debe ser único y solo letras, números, `.`, `_`, `-`. `path` relativo se resuelve desde el directorio del manifest. Si el directorio no existe, el comando aborta con error.

### Aviso clave sobre repos independientes

**Cada raíz es un repositorio independiente.** Engrafo nunca hace push ni commit. El agente recibe la ruta y el remote de cada raíz en el mensaje de sesión (via el hook `session-start`), y debe hacer `cd` a la raíz correcta antes de ejecutar `git commit` o `git push`. Ejemplo: si el workspace tiene `backend-auth` en `./services/auth`, el agente debe `cd services/auth && git commit` — no desde la raíz del workspace.

---

## Sin git

Engrafo funciona sin `.git`. En ese caso la raíz se registra con `vcs=none` y los cambios se detectan por **checksum SHA-256** del contenido de cada archivo:

- Cada `engrafo update` compara el checksum actual de cada archivo con el indexado. Si hay cambios, crea una revisión `source=checksum` y actualiza solo los archivos modificados.
- El hook `session-start` dispara un update automático al inicio de cada sesión, por lo que los cambios quedan registrados sin intervención manual.

**Limitaciones honestas vs. git:**
- Sin git no hay `--from-git` (no se puede replay del pasado anterior al primer `init`).
- Los cambios realizados con engrafo apagado y sin ejecutar `update` no quedan en el historial.
- La granularidad de la historia es "por update", no por commit.

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

Engrafo se integra con [engram](https://github.com/Gentleman-Programming/engram), el sistema de memoria episódica del ecosistema Gentle-AI. La integración es **opcional pero recomendada**: sin ella las herramientas estructurales funcionan completas, pero (cg_anchor) y el contenido de observaciones en (cg_history) quedan inactivos.

**engram se instala automáticamente** al ejecutar (engrafo hooks install). Si ya tienes Gentle-AI configurado, engram ya está instalado.

### Auto-anclaje (el puente que une ambos sistemas)

El hook `PostToolUse` sobre `mcp__engram__mem_save` ejecuta `engrafo hook post-mem-save`: cada vez que el agente guarda una memoria en engram, engrafo extrae el `sync_id` de la observación y la **ancla automáticamente** a los nodos del grafo cuyos símbolos se mencionan en el título/contenido.

El matching es **case-sensitive exacto**: la prosa va en minúscula (`decidimos`, `usar`) y no colisiona con símbolos de código (`AuthService`, `handler.ts`), así que el ruido se filtra estructuralmente; solo un símbolo que existe como nodo en el grafo produce un anclaje. Resultado: al consultar `cg_node UserService` ves las decisiones de engram ancladas a ese símbolo, sin que el agente tenga que llamar `cg_anchor` a mano.

El namespace de engram se fija con `--project <workspace>` en `.mcp.json` (generado por `hooks install`), evitando la detección ambigua de `.git` en workspaces multi-repo.

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

| Comando                                   | Descripción                                                                                     |
|-------------------------------------------|-------------------------------------------------------------------------------------------------|
| `workspace add <name> <path> [flags]`     | Registra una raíz en el workspace (crea `engrafo.json` si no existe) e indexa inmediatamente.  |
| `workspace list`                          | Lista raíces registradas con ruta, VCS, remote, nodos indexados y fecha.                        |
| `workspace remove <name>`                 | Elimina la raíz del manifest y borra sus nodos/aristas de la BD (cascada).                      |
| `init [root]`                             | Indexa el repositorio o workspace desde cero. Crea `.engrafo/graph.db`.                        |
| `init --from-git N [root]`                | Replaya los últimos N commits para construir historia bi-temporal (solo raíces con git).        |
| `update [--root <name>]`                  | Actualización incremental. Sin `--root`: todas las raíces. Con `--root`: solo esa.             |
| `serve`                                   | Inicia el servidor MCP en stdio. Usado por la configuración del agente.                         |
| `ui [--port N]`                           | Abre el navegador de grafo en `http://localhost:8080` (defecto: 8080).                          |
| `deadcode [--json] [--threshold-days N]`  | Lista candidatos a dead code (huérfanos y abandonados).                                         |
| `status`                                  | Muestra estadísticas del índice: nodos, aristas, raíces, último commit por raíz.               |
| `query <symbol>`                          | Consulta un símbolo directamente desde la CLI.                                                  |
| `hooks install`                           | Instala hooks de Claude Code en `.claude/settings.json`.                                        |
| `hooks uninstall`                         | Elimina los hooks instalados.                                                                    |
| `doctor`                                  | Verifica la instalación: BD, schema, soporte FTS5.                                              |

**Flag global:**

`--db <ruta>` — ruta explícita al archivo `graph.db`. Por defecto se detecta automáticamente: primero busca `engrafo.json` subiendo por padres, luego `.git`, luego usa el directorio actual.

**Flags de `workspace add`:**

```sh
--remote <url>     # URL remota del repo (se auto-detecta de git origin si no se pasa)
--branch <branch>  # Rama por defecto (se auto-detecta de HEAD si no se pasa)
--vcs git|none     # Fuerza el tipo de VCS (se auto-detecta si no se pasa)
```

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
Detecta código muerto. Solo evalúa `function`, `class` e `interface` — los `method` se excluyen porque se invocan sobre instancias, no por nombre, y no pueden tener aristas entrantes bajo el modelo actual.

- **Huérfanos:** nodos que nunca tuvieron ninguna arista entrante (`uses`, `calls`, `inherits`).
- **Abandonados:** nodos que tenían aristas entrantes y las perdieron todas. El modelo bi-temporal hace esta detección altamente precisa — distingue "nunca usado" de "fue usado y dejó de serlo".

```json
{ "threshold_days": 30 }   // opcional: solo inactivos por más de N días
```

> **Nota sobre entrypoints externos:** funciones invocadas por infraestructura (AWS Lambda `lambda_handler`, webhooks, cron) aparecen como huérfanas porque ningún código interno las llama. Son falsos positivos esperados que el agente debe interpretar con contexto.

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
Normalmente **no necesitas llamarlo a mano**: el hook `post-mem-save` ancla
automáticamente cada `mem_save` (ver [Auto-anclaje](#auto-anclaje-el-puente-que-une-ambos-sistemas)).
Úsalo solo para anclar una observación existente a símbolos adicionales.

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

Interfaz local read-only con tres pestañas:

- **Graph** — lista de símbolos con búsqueda FTS5 y panel de detalle: aristas activas, históricas e IDs de engram anclados.
- **Grafo** — visualización force-directed del grafo completo. Permite:
  - Filtrar por raíz (selector en la barra superior; útil en workspaces multi-repo).
  - Arrastrar nodos, hacer zoom con la rueda del ratón, pan con arrastre del fondo.
  - Click en un nodo para ver su detalle en el panel lateral.
  - Color de relleno por tipo de símbolo; borde tintado por raíz (cada raíz recibe un color distinto).
- **Dead Code** — huérfanos (nunca tuvieron referencias) y abandonados (las perdieron).

No requiere internet. No instala dependencias adicionales (el HTML y todo el JS están embebidos en el binario).

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

- **Nodos:** `file`, `function`, `class`, `method`, `interface`, `package`, `external`.
- **Aristas:** dirigidas, con `valid_from_rev` para la aparición y `valid_until_rev` para la desaparición. Nunca se eliminan — se invalidan. Tipos emitidos por los extractores:

| Tipo | Semántica | Ejemplo |
|------|-----------|---------|
| `imports` | archivo depende de módulo/archivo | `auth.ts → ./utils` |
| `uses` | archivo referencia símbolo por nombre (named import) | `auth.ts → AuthService` |
| `calls` | símbolo es invocado dentro del mismo archivo | `handler.py → validate_input` |
| `inherits` | clase extiende otra | `AdminUser → User` |

- **Anchors:** tabla `engram_anchors` vincula `engram_obs_id` → `node_id`.

### Migración v1 → v2

Al abrir una base de datos v1 con un binario v2, **la migración ocurre automáticamente** en la primera ejecución. El proceso:

1. Crea las tablas `roots` y `revisions`.
2. Registra el repo existente como raíz #1.
3. Mapea los hashes de commit a revisiones.
4. Añade `root_id` a todos los nodos.
5. Reconstruye la tabla `edges` con las columnas `valid_from_rev`/`valid_until_rev`.

**Se recomienda hacer una copia de `graph.db` antes de la primera ejecución** para tener un punto de restauración. La migración preserva todos los nodos, aristas (activas e históricas) y anchors.

### CGO

| Funcionalidad              | CGO requerido | Sin CGO                                  |
|---------------------------|---------------|------------------------------------------|
| Parser tree-sitter        | **Sí**        | `init` corre pero no extrae símbolos     |
| SQLite (modernc)          | No            | Funciona completo                        |
| MCP server                | No            | Funciona completo                        |
| UI server / grafo visual  | No            | Funciona completo                        |

La distribución oficial se compila con CGO. Sin CGO el binario arranca y todos los comandos excepto la extracción de símbolos funcionan — útil para CI, tests y ambientes sin compilador C.

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

**Nota para Windows:** TDM-GCC 10.x rompe CGO con Go 1.25.x (binarios que Windows rechaza). Usar MSYS2 mingw64 GCC 16+ o w64devkit. Ver sección "CGO en Windows" en Instalación. Para tests y CI sin parser tree-sitter: `CGO_ENABLED=0 go test ./... -count=1`.

---

## Licencia

MIT — ver [LICENSE](LICENSE).
