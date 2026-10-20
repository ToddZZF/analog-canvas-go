# Local Go server contract

Status: `adopted`

Primary owner: `apps/local-server`

The Go server in `apps/local-server` is the single-binary local
distribution: it serves the offline editor bundle and implements the
Electron desktop host API from `apps/desktop/src` over local files. The
editor's local composition (`apps/editor/src/hosts`, the `local-web`
services) is its only API consumer; hosted cloud surfaces (`/api/*`) do
not exist here. Desktop behaviors this contract ports keep their
authority in the apps/desktop sources named below.

## Rules

- One circuit is one `<uuid>/project.icproj.json` directory under a
  library root; favorites live in `.project-meta.json`, prior saves in
  `.history/` (at most 3, pending-journal recovery per
  `apps/desktop/src/project-history.ts`), deletions in `.recycle/`.
- Server state files sit in the data directory using the desktop
  preview's names: `project-library.json`, `recent-projects.json` plus
  `.authorized`/`.creations` suffixes, `external-history.json` plus
  `external-history/`, `components.json`. Pointing `--data-dir` at a
  desktop-preview userData directory migrates it.
- Exactly one `/desktop/project/*` operation runs at a time; saves commit
  through temp file + fsync + rename after a byte-digest overwrite check,
  and a failed first allocation leaves its directory retriable, never
  half-written.
- Write routes accept only the served loopback origins; every other
  origin gets 403.
- Static serving follows `apps/desktop/src/app-protocol.ts`: CSP with
  sha256-hashed inline bootstrap scripts plus `wasm-unsafe-eval`,
  `cross-origin-opener-policy: same-origin`, `nosniff`, no-cache document,
  immutable hashed assets, `/assets/*` misses 404, extensionless paths
  answered by `index.html`.
- The browser host cannot host native dialogs: Save As answers
  `cancelled` outside the project library, Open Project uses a
  content-based open that imports an independent library copy, and
  destructive library actions confirm inside the page unless the desktop
  bridge (`__analogCanvasDesktop`) is present.
- `/desktop/export` and `/api/*` are absent; exports download through the
  browser page. `/desktop/system/info` reports `packaged: false`, which
  keeps file-association controls disabled.
- The tray offers open-editor, open-projects-folder and quit. On Unix it
  hosts through fyne.io/systray's StatusNotifier protocol when a
  StatusNotifierWatcher is on the session bus, and through the GTK XEmbed
  status icon (`tray_xembed.c`) when an X11 display exists without one;
  panels hosting neither stay in console mode. Builds with `-tags notray`
  omit the tray and run as a pure-Go console server. Panels that host
  neither protocol quit through the editor's File menu instead.

## Contract

- `POST /desktop/project/{next-launch,resume,open,save,recent,forget,release,library,library-action,history,library-directory,copy-to-library,saved-copy,reveal}`
  with the request/response shapes of `apps/desktop/src/project-files.ts`.
  Receipt: `{id,name,path,revision,byteDigest}`; new bindings start at
  revision 1; conflicts answer `{status:"conflict",message}`.
- `POST /desktop/components/{list,read,save,delete,restore,check}` with
  the shapes of `apps/desktop/src/component-library.ts`; entries are
  opaque JSON revalidated by the renderer's shared parser; at most 500
  entries and 16 MiB per library file.
- `POST /desktop/system/info` and `POST /desktop/external` with the
  shapes of `apps/desktop/src/system-info.ts` and `external-links.ts`.
- `POST /desktop/quit` (no body) answers `{status:"quitting"}` and then
  runs the same graceful shutdown as tray Quit and Ctrl+C. It accepts
  only the served loopback origins, and the editor confirms inside the
  page before calling it.

## Acceptance criteria

- The `go test ./...` suite in `apps/local-server` pins project flows
  (save/open/resume conflicts, history restore/branch, creation replay,
  recycle/purge), pending-journal recovery, atomic-failure cleanup,
  component lifecycle, and the static serving contract.
- `pnpm build:local` produces a binary whose editor answers
  Ctrl+S by writing the project file under the data directory; the e2e
  editor suite stays green for the shared hosts code it covers.
