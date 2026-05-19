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

Seed scaffold. The first implementation target is a read-only local session
index over `~/.codex/sessions`, followed by an interactive picker.

## Development

```sh
just fmt
just test
just lint
just build
```

