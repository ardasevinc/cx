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
cx --no-alt-screen
```

Keyboard:

- type to search
- `j`/`k` or arrows to move
- `enter` to run `codex resume <session-id>`
- `ctrl+f` to run `codex fork <session-id>`
- `tab` toggles preview
- `ctrl+e` toggles detail view
- `ctrl+v` toggles compact/comfy rows
- `esc`/`ctrl+c` exits

## Development

```sh
just fmt
just test
just lint
just build
```
