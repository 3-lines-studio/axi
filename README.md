# Axi

Native and web graphical client for a remote Axis server.

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

## Web

Build Axi, then run its web server:

```sh
wails3 build
AXI_AXIS_URL=http://127.0.0.1:8081 \
AXI_AXIS_USERNAME=axbot \
AXI_AXIS_PASSWORD=change-me \
bin/axi web
```

Axi listens on `127.0.0.1:8080` by default. Set `AXI_WEB_ADDRESS` to change it. The web server owns the frontend and proxies API and event streams to Axis, so Axis stays API-only and its credentials do not reach the browser.

From another machine, keep Axi bound to localhost and use an SSH tunnel:

```sh
ssh -L 8080:127.0.0.1:8080 server
```

Then open `http://127.0.0.1:8080`.

## systemd

Axi includes user services for Axis and `axi web`. The Makefile expects the Axis repository at `../axis` by default. Override `AXIS_DIR` when it is elsewhere.

Create the shared environment file:

```sh
make configure
$EDITOR ~/.config/axis/environment
```

Set a strong matching username and password for Axis and Axi:

```text
AXIS_ADDRESS=127.0.0.1:8081
AXIS_USERNAME=axbot
AXIS_PASSWORD=replace-this
AX_TOOLS=fsx bashx skillx
AXI_AXIS_URL=http://127.0.0.1:8081
AXI_AXIS_USERNAME=axbot
AXI_AXIS_PASSWORD=replace-this
AXI_WEB_ADDRESS=127.0.0.1:8080
```

Build, install, and start both services:

```sh
make install
```

After changing either checkout, rebuild, replace both binaries, and restart both services:

```sh
make update
```

`make update` uses the current local source. It does not fetch or change Git branches.

Service commands:

```sh
make status
make logs
make restart
make stop
make uninstall
```

The services run under the current user. Their units are installed under `~/.config/systemd/user`, binaries under `~/.local/bin`, and credentials remain in the mode-600 environment file. `make uninstall` leaves that environment file and all Axis data intact.

To start user services after boot without an interactive login:

```sh
loginctl enable-linger "$USER"
```

## Check

```sh
npm --prefix frontend run check
npm --prefix frontend run build
go test ./...
wails3 build
```

The Linux binary is written to `bin/axi`.
