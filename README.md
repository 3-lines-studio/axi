# Axi

Native and web graphical interface for AX.

## Install

Install Axi and its core local runtime:

```sh
curl -fsSL https://ax.3lines.studio/install.sh | sh -s -- axi
axi
```

The bundle includes `axi`, `axis`, `ax`, `fsx`, `bashx`, `skillx`, and `attachx`. Open `http://127.0.0.1:8080` after startup. On first launch, Axi asks for an API key, model, and optional OpenAI-compatible endpoint. Local providers may omit the key. Axi stores the AX configuration locally with mode `0600` and never returns a saved key to the browser.

Running `axi` starts a detached local Axi when needed and opens it in the default browser. Later invocations reuse the same process. Axi and Axis keep running after the terminal and browser close. Logs are stored under `~/.local/share/axi/logs`.

Update by running the install command again. Running binaries are replaced atomically. Uninstall the complete binary bundle with:

```sh
curl -fsSL https://ax.3lines.studio/install.sh | sh -s -- uninstall axi
```

Uninstall keeps Axi data and AX configuration. Remove them separately only when the data is no longer needed.

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

Build Axi, then start the local stack:

```sh
wails3 build
bin/axi web
```

The released headless binary starts a persistent local Axi and opens the browser when invoked as plain `axi`. The explicit `web` form remains for existing services and foreground deployments.

When `AXI_AXIS_URL` is absent, Axi starts a private local Axis process and stops it on shutdown. Local state lives under `~/.local/share/axi`; set `AXI_HOME` to change it. The `axis` and `ax` binaries must be on `PATH`. On first start, Axi creates a default project for the current directory and an Assistant bot with `fsx`, `bashx`, `skillx`, and `attachx`. Set `OPENAI_API_KEY` or configure AX before starting Axi.

To use an existing local or remote Axis instead:

```sh
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

## Docker

Run the all-in-one image on localhost with persistent data:

```sh
docker run --rm \
  -p 127.0.0.1:8080:8080 \
  -e AXI_ALLOW_PUBLIC=true \
  -v axi-data:/data \
  ghcr.io/3-lines-studio/axi
```

Build it locally from the Axi repository with `docker build -t axi .`.

Open `http://127.0.0.1:8080` and complete provider setup. The image runs as a non-root user and includes Axi, Axis, AX, Fsx, Bashx, Skillx, and Attachx. Binary versions are pinned and their release checksums are verified during the build.

Axi refuses non-loopback listeners unless `AXI_ALLOW_PUBLIC=true`. That flag only permits the listener; it does not add authentication. Keep localhost publishing for personal use. Put any internet-facing deployment behind an authentication proxy.

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
