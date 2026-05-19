# cx

`cx` is a fast local Codex session picker for the terminal.

It reads Codex's local session state, shows all previous threads sorted by last
updated, supports immediate search and preview/detail views, then delegates
continuation back to Codex:

```sh
codex resume <session-id>
codex fork <session-id>
```

## Install

```sh
go install github.com/ardasevinc/cx/cmd/cx@latest
```

Requirements:

- Codex CLI available as `codex`.
- Local Codex state under `~/.codex`.
- `sqlite3` on `PATH` for the fast state-db path. `cx` falls back to JSONL scans
  if unavailable.

## Use

```sh
cx
cx --list --limit 20
cx list --limit 20
cx version
cx -V
cx --no-alt-screen
```

`cx --help` prints the full CLI, flag, key, and command reference.

Keyboard:

- type to search
- arrows, `ctrl+j`/`ctrl+k`, mouse wheel, page keys, home/end to move
- `enter` to run `codex resume <session-id>`
- `ctrl+f` to run `codex fork <session-id>`
- `y` copies the selected session id
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
:resume
:fork
:copy id|path|cwd|title|resume|fork
:view compact|comfy
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
