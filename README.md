# AX UI

Cross-platform Wails client for a remote Axis server.

## Requirements

- Go 1.25 or newer
- Node.js and npm
- Wails v3.0.0-beta.12
- Linux WebKitGTK development packages

Install the pinned Wails CLI:

```sh
GOBIN="$HOME/.local/bin" go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12
```

## Develop

```sh
npm --prefix frontend install
wails3 dev
```

Enter the Axis server URL and its Basic Auth credentials on the connection screen. Projects and sessions come from Axis. Configure project paths in Axis's `projects.json`; see the Axis README.

## Check

```sh
npm --prefix frontend run check
npm --prefix frontend run build
go test ./...
wails3 build
```

The Linux binary is written to `bin/axi`.
