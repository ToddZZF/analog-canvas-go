# Analog Canvas Local (Go server)

One Go binary serves the Analog Canvas editor and stores every project on
the local disk. No account, cloud, gallery, Agent or hosted simulation is
involved: those modules are compiled out of the editor bundle by the same
policy the desktop preview uses (`ICM_DESKTOP` / `desktop-module-evidence`).

## Run

```powershell
pnpm build:local          # editor bundle + Go binary (tray, cgo)
./apps/local-server/dist/analog-canvas-local
```

The server listens on `http://127.0.0.1:4173` by default, opens the system
browser, and puts a tray icon in the desktop panel with **Open Editor**,
**Open Projects Folder**, **About** and **Quit**.

| Flag             | Default              | Meaning                                          |
| ---------------- | -------------------- | ------------------------------------------------ |
| `--addr`         | `127.0.0.1:4173`     | Listen address; an occupied default port falls back to a free one. |
| `--data-dir`     | `~/Analog Canvas`    | Root for projects, settings and the component library. |
| `--projects-dir` | `<data-dir>/Projects` | Project library root.                            |
| `--no-tray`      | off                  | Console-only server (Ctrl+C quits).              |
| `--no-open`      | off                  | Do not open the browser on startup.              |

`-tags notray` (`pnpm build:local:notray`) builds a pure-Go binary with the
tray removed: CGO-free, works headless over SSH. On Linux the tray uses the
DBus StatusNotifier/AppIndicator protocol — KDE shows it natively, while
GNOME needs the "AppIndicator and KStatusNotifierItem Support" extension.
Without a session bus the server logs and stays in console mode.

## Where data lives

```
~/Analog Canvas/
├── Projects/<uuid>/project.icproj.json   one circuit per directory
│   ├── .project-meta.json                favorite flag
│   └── .history/                         up to 3 prior saves + manifest
├── .recycle/<uuid>/                      recoverable deletions
├── components.json                       User Components library
├── project-library.json                  library roots and active root
├── recent-projects.json(.authorized/.creations)  recents, grants, receipts
└── external-history.json(+)              history index for external files
```

Projects are plain `.icproj.json` files; you can back them up, diff or edit
them outside the app. The state-file names and layout match the Windows
desktop preview's application-data directory, so an existing preview
installation migrates by pointing both flags at it:

```powershell
analog-canvas-local --data-dir "%APPDATA%\Analog Canvas" --projects-dir "%USERPROFILE%\Analog Canvas\Projects"
```

The server contract itself is specified in
[docs/specs/local-server.md](../../docs/specs/local-server.md).

## Saving

**Ctrl+S / File → Save** writes into the project library. **Save As…**
creates an independent project in the library (a browser page has no native
file dialog, so the "choose another file" destination answers as
cancelled). **File → Open Project…** uses the browser's file picker and
imports the chosen `.icproj.json`/`.icproj` as an independent library
project. Destructive library actions (recycle, purge, version restore)
confirm inside the page; the desktop shell confirms natively instead.

Exports (SVG/PNG/PDF, SPICE/Spectre netlists) download through the browser.
"Show in folder" and the fixed product links open in the system file
manager / browser via the server, which shares your desktop session.

## Building for release

```powershell
pnpm build:local:release
```

produces `apps/local-server/dist/`:

- `analog-canvas-local-linux-amd64` — tray build (built on this machine).
- `analog-canvas-local-{linux,windows,darwin}-{amd64,arm64}-notray` —
  pure-Go console builds, cross-compiled.
- `analog-canvas-local-windows-amd64.exe` — tray build, only when
  `x86_64-w64-mingw32-gcc` is installed; otherwise build it on Windows with
  `pnpm build:local`.

macOS tray builds need a Mac (`pnpm build:local` there). The tray library
(`fyne.io/systray`) requires cgo on every platform.

## Development

```powershell
cd apps/local-server
go test ./...
go vet ./...
```

The module is stdlib plus `fyne.io/systray`. The HTTP contract mirrors
`apps/desktop/src/` (the Electron host): `project-files.ts` for
`/desktop/project/*`, `component-library.ts` for `/desktop/components/*`,
`system-info.ts`/`external-links.ts` for the system surface, and
`app-protocol.ts` for static serving headers. The embedded bundle in `web/`
is build output; `pnpm build:local` refreshes it from
`apps/editor/dist-local` (the `vite.local-web.config.ts` composition).
