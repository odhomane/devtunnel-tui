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

## Installation

### Download prebuilt binaries (recommended)

1. Open the latest release:
   - https://github.com/odhomane/devtunnel-tui/releases/latest
2. Download the asset that matches your platform:
   - `devtunnel-tui_<version>_darwin_amd64.tar.gz`
   - `devtunnel-tui_<version>_darwin_arm64.tar.gz`
   - `devtunnel-tui_<version>_linux_amd64.tar.gz`
   - `devtunnel-tui_<version>_linux_arm64.tar.gz`
3. Extract and move the binary into your `PATH`.

macOS example:
```bash
curl -L -o devtunnel-tui.tar.gz https://github.com/odhomane/devtunnel-tui/releases/latest/download/devtunnel-tui_vX.Y.Z_darwin_arm64.tar.gz
tar -xzf devtunnel-tui.tar.gz
sudo mv devtunnel-tui_darwin_arm64/devtunnel-tui /usr/local/bin/devtunnel-tui
```

Linux example:
```bash
curl -L -o devtunnel-tui.tar.gz https://github.com/odhomane/devtunnel-tui/releases/latest/download/devtunnel-tui_vX.Y.Z_linux_amd64.tar.gz
tar -xzf devtunnel-tui.tar.gz
sudo mv devtunnel-tui_linux_amd64/devtunnel-tui /usr/local/bin/devtunnel-tui
```

### Build from source

```bash
git clone https://github.com/odhomane/devtunnel-tui.git
cd devtunnel-tui
go build -o bin/devtunnel-tui .
./bin/devtunnel-tui
```

## Run (dev)

```bash
go mod tidy
go run .
```

## Controls

- `h/l` or `←/→`: switch category
- `j/k` or `↑/↓`: move command selection
- `1..6`: jump directly to a resource category
- `enter`: run selected command
- `:`: open command mode (type raw command after `devtunnel`)
- `/`: filter commands in current category
- `u/d` or `PgUp/PgDn`: scroll output
- `r`: rerun last command
- `q`: quit
- Form mode:
  - `Enter`: next field / run
  - `Esc`: cancel

## Notes

- This app wraps the official `devtunnel` binary. It does not reimplement protocol behavior.
- For advanced or newly added CLI subcommands, use the `custom` command entry.

## Release automation

- GitHub Actions builds release assets for:
  - `darwin/amd64`
  - `darwin/arm64`
  - `linux/amd64`
  - `linux/arm64`
- A release is automatically published when you push a tag such as `v0.1.0`.
