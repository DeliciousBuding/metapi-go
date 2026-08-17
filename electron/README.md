# Metapi Desktop

A minimal Electron wrapper that turns the Metapi Go server + its web admin UI into a desktop app with a system-tray icon.

This is a **thin shell** — it does not reimplement the original TypeScript Electron app's full feature set. It only:

- spawns and manages the `metapi` Go binary (`metapi.exe` on Windows),
- opens a single `BrowserWindow` that loads the Go admin UI at `http://127.0.0.1:4000`,
- shows a "waiting for server" screen and retries every 2s until the server answers `/ready`,
- provides a system-tray icon with **Open Admin / Start Server / Stop Server / Launch at login / Quit**,
- surfaces native OS notifications by polling the server's notifications endpoint (best-effort, see below).

The Electron project is **independent of Go** — it lives in `electron/` and is pure Node.js. Building it does not touch the Go module.

## Layout

```
electron/
  package.json     # electron + electron-packager devDeps, npm scripts
  main.js          # main process: window, tray, Go-binary lifecycle, notifications
  preload.js       # contextBridge exposing appVersion/platform + notification IPC
  tray-icon.png    # placeholder tray icon (copied from web/public/desktop-icon.png)
  README.md        # this file
```

## Prerequisites

- Node.js >= 18 (for running Electron + electron-packager).
- Go toolchain (only needed when (re)building the `metapi` binary).
- The React admin UI must already be built into `web/dist/` (the Go binary embeds it via `go:embed`). Run `make web-build` first if `web/dist/` is missing.

## Build

From the repo root:

```bash
# cross-platform (bash)
make electron-build

# …or run the script directly
scripts/build-electron.sh

# Windows PowerShell
scripts/build-electron.ps1
```

The build script will:

1. ensure `web/dist/` exists (runs `make web-build` if missing — requires Bun),
2. `go build -o electron/metapi ./cmd/server` (produces `electron/metapi` or `electron/metapi.exe`),
3. `cd electron && npm install`,
4. `electron-packager …` → output in `electron/dist/`.

> If you only want to iterate on the shell without rebuilding the SPA/Go binary every time, build the Go binary once (`go build -o electron/metapi ./cmd/server`) and then run `cd electron && npm start`.

## Run (dev)

```bash
cd electron
npm install
npm start          # launches electron . — spawns ./metapi (or ./metapi.exe) and opens the window
```

By default the desktop app auto-spawns the Go binary from the `electron/` directory. To point it at a different binary, set `METAPI_BINARY=/absolute/path/to/metapi`.

## Environment variables

| Variable | Purpose |
|---|---|
| `METAPI_BINARY` | Override the Go binary path used by the shell. |
| `METAPI_DESKTOP_NOTIFY_TOKEN` | Bearer token for the admin notifications endpoint. When unset (default), notification polling is skipped to avoid 401 spam on the auth-gated admin API. |
| All others | Inherited from the Electron process and passed through to the spawned Go binary (e.g. `PORT`, `AUTH_TOKEN`, …). |

## Notifications

The admin API is auth-gated, so the desktop shell only polls `/api/admin/notifications` when `METAPI_DESKTOP_NOTIFY_TOKEN` is set. A `404` (endpoint not implemented yet) disables polling for the rest of the session. When the endpoint returns a JSON array (or an object with `items`/`notifications`), each unseen item is shown as a native OS notification and forwarded to the renderer via the preload `metapiDesktop.onNativeNotification` bridge.

## Packaging

`npm run package` (in `electron/`) runs `electron-packager` and emits `electron/dist/<platform>-<arch>/Metapi-Desktop*`. Override platforms/arches by editing the `package` script in `electron/package.json`.

## Notes

- The window closing does **not** quit the app — the tray icon and the Go server keep running. Use **Quit** from the tray to stop the server and exit.
- On Windows, `Stop Server` uses a hard process-tree kill (Node's `SIGTERM` maps to `taskkill`), which is acceptable for a stop action.
- Logs from the spawned Go server are appended to `<logs dir>/metapi-server.log` (`%APPDATA%/Metapi/logs` on Windows, `~/Library/Logs/Metapi` on macOS, `~/.config/Metapi/logs` on Linux).
