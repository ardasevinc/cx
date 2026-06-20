# cx

`cx` is a fast local Codex session picker for the terminal.

It reads Codex's local session state, shows all previous threads sorted by last
updated, supports immediate search and preview/detail views, then delegates
continuation back to Codex:

```sh
codex --yolo -C <session-cwd> resume <session-id>
codex --yolo -C <session-cwd> fork <session-id>
```

## Install

```sh
go install github.com/ardasevinc/cx/cmd/cx@latest
```

Requirements:

- Codex CLI available as `codex`.
- Local Codex state under `~/.codex`.

## Privacy and compatibility

`cx` is local-only. It reads Codex session metadata/transcripts from your
machine and launches the `codex` CLI; it does not upload session data or call a
remote service.

Codex's local storage format is not a stable public API. `cx` follows the
currently observed `~/.codex/state_5.sqlite`, `~/.codex/session_index.jsonl`,
and rollout JSONL formats, so future Codex releases may require compatibility
updates.

## Use

```sh
cx
cx here
cx --cwd ~/programming/open-source/cx
cx --list --limit 20
cx list --here --limit 20
cx list --cwd ~/programming/open-source/cx --limit 20
cx new
cx new "debug oauth staging"
cx new --cwd ~/programming/open-source/cx
cx index status
cx index refresh
cx index rebuild
cx doctor
cx update --check
cx update
cx version
cx -V
cx --no-alt-screen
```

`cx --help` prints the full CLI, flag, key, and command reference.

`cx here` filters the picker to recent threads relevant to the current shell
directory. It prefers exact cwd matches, then sessions from the same detected
git root, so running it from `repo/apps/web` still shows threads started from
`repo`, `repo/apps/web`, or sibling repo directories. Use `cx --cwd DIR` to
scope to another directory, or `cx list --here` / `cx list --cwd DIR` for
noninteractive output. Inside the TUI, `:here` toggles this current cwd/project
scope on and off.

`cx index refresh` incrementally builds the local transcript cache at
`~/.cache/cx/index.sqlite`. The cache is disposable; Codex's own
`~/.codex/state_5.sqlite` and rollout JSONL files remain authoritative.
Transcript search and richer previews use the cache when available and degrade
to metadata-only behavior when it is empty or unavailable.

`cx index status --json` and `cx doctor --json` expose cache health, FTS
availability, missing/failed/truncated sessions, cache size, and storage-shape
compatibility checks for automation.

`cx new [name]` creates a fresh local chat directory under
`~/Documents/Codex/YYYY-MM-DD/` using the local date, then launches:

```sh
codex --yolo -C <created-dir>
```

If no name is provided, it creates the next `chat-001`, `chat-002`, ...
directory. Named chats are slugged and collision-free, so an existing folder is
never overwritten. `cx new --cwd DIR` starts a fresh Codex thread in an existing
project directory with the same `--yolo -C` launch contract.

`cx update --check` compares the current binary to the latest GitHub tag.
`cx update` installs the latest tagged release with:

```sh
go install github.com/ardasevinc/cx/cmd/cx@<latest-tag>
```

Keyboard:

- type to search
- arrows, `ctrl+j`/`ctrl+k`, mouse wheel, page keys, home/end to move
- left/right close/open the selected group in grouped views
- `enter` resumes a session, starts the selected new chat/project row, or toggles a group
- `ctrl+n` starts a fresh Codex thread in the selected session/project context
- `ctrl+p` opens the project launcher
- `ctrl+g` opens grouped projects
- `ctrl+f` to run `codex fork <session-id>`
- `ctrl+y` copies the selected session id
- `:` opens command mode
- `?` opens help
- `tab` toggles preview
- `ctrl+e` toggles detail view
- `ctrl+v` toggles compact/comfy rows
- `esc`/`ctrl+c` exits

Plain `j` and `k` enter the search query. The direct vim-ish movement keys are
kept on `ctrl+j` and `ctrl+k` so search stays immediate.

Command mode:

```text
:new [name]
:resume
:fork
:copy id|path|cwd|title|resume|fork
:view all|chats|projects|grouped|compact|comfy
:group projects|chats
:here
:sort date|source
:open | :close | :toggle | :open-all | :close-all
:preview
:detail
:clear
:quit
```

## Development

```sh
just gate
just fmt
just test
just lint
just build
```

## License

MIT
