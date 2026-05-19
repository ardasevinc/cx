# cx

`cx` is a custom Codex session picker for Arda's CLI workflow.

The intended shape is a fast, searchable TUI that indexes previous Codex threads,
groups them by project and general chat context, previews transcripts, and then
delegates actual continuation to Codex:

```sh
codex resume <session-id>
codex fork <session-id>
```

## Status

Early local build. `cx` reads Codex's state database when available, falls back
to local session JSONLs, and opens a searchable picker that can resume or fork
the selected thread.

## Use

```sh
cx
cx --list --limit 20
cx list --limit 20
cx version
cx -V
cx --no-alt-screen
```

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
