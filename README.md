# DevTunnels TUI

Fast terminal UI for `devtunnel` command management on macOS and Linux.

## What it does

- Full top-level command coverage from `devtunnel`:
  - `list`, `show`, `create`, `update`, `delete`, `delete-all`
  - `token`, `set`, `unset`, `access`, `user`, `port`
  - `host`, `connect`, `limits`, `clusters`, `echo`, `ping`
- Authentication actions:
  - `user login`
  - `user logout`
- Raw custom mode to run any command after `devtunnel` (covers subcommands/options not explicitly modeled).
- Interactive prompts for required parameters.
- Async command execution with streaming output pane and rerun-last support.

## Requirements

- Go 1.22+
- `devtunnel` CLI installed and available in `PATH`

## Run

```bash
go mod tidy
go run .
```

## Controls

- `j/k` or arrow keys: select command
- `Enter`: execute selected command
- `r`: rerun last command
- `?`: toggle help
- `q`: quit
- In prompt mode:
  - `Enter`: next field / run
  - `Esc`: cancel

## Notes

- This app wraps the official `devtunnel` binary. It does not reimplement protocol behavior.
- For advanced or newly added CLI subcommands, use the `custom` command entry.
