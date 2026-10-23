# Note Host

Status: `accepted`

Primary owner: `apps/editor/src/hosts/obsidian-bridge.ts` (protocol),
`apps/editor/src/entries/obsidian.tsx` (editor frame),
`apps/editor/src/entries/render.tsx` (figure frame),
`apps/editor/src/features/circuit-region/region-target.ts` (named regions)

The circuit editor runs inside a note application — Obsidian, in the repository that
owns it. The note application owns the file: it hands over the Project's text and
writes back the text the editor returns. This spec owns the contract between the
two sides and the bundle that carries them. The Project file itself stays with
[`project-file-format.md`](project-file-format.md), the formal scene with
[`visual-language.md`](visual-language.md) and the exported figure with
[`export.md`](export.md).

## Rules

- One build produces both frames: `obsidian.html` (the editor) and `render.html`
  (the figure). `pnpm build:obsidian` writes both into `dist-obsidian`.
- The note bundle carries the same global stylesheet as the desktop bundle. The
  height chain `body` → `#root` → `.app-shell` → `.app-workspace` →
  `.canvas-panel` is what gives the canvas a definite size.
- `.app-workspace` holds exactly one row of `minmax(0, 1fr)`. The canvas fills
  the frame it is given: a row sized by its content would scale the drawing to a
  canvas that overflows the note.
- The editor never touches the disk. It receives the Project's text and returns
  the text to keep.
- Every message carries the channel name `analog-canvas`. A frame ignores a
  message on any other channel.
- A frame answers only the messages of its own host: a listener checks the
  message source, so the editor frame and the figure frame of the same window
  never answer each other.
- The editor frame shows one drawing. The drawing replaces the blank tab the
  frame opens with.
- The editor frame posts `ready` after it mounts; the figure frame posts `ready`
  before it answers its first request.
- A region is a Rectangle Annotation named by the drafting text anchored to it.
  A name that no annotation carries leaves the view on the whole sheet.
- The drawing is a note, so it is written like one: the host says how eagerly
  it wants the drawing written, and the editor writes it after the last edit.
  A host that says nothing keeps the editor's own explicit save.
- The editor says when the drawing holds changes that are not on disk, so the
  host can show that the drawing wants to be saved.
- The host's own save command reaches the drawing: the host asks the editor
  to write, and the editor writes it the way a person pressing Save does.
- The note composition excludes the community, Agent, simulation and cloud
  surfaces. The Project library, the recent list and the drawing itself stay.
- A note frame shows the drawing the note application handed over and nothing
  else: it offers neither a new nor an imported Project, and a drawing that is
  bound to no file is never written. Writing the hand-over path anyway would
  destroy the drawing the note still points at.

## Contract

Channel `analog-canvas`.

Note application → editor frame:

| Message       | Meaning                                                                   |
| ------------- | ------------------------------------------------------------------------- |
| `open`        | Hands over a Project: `path`, `text`, and the `focus` region a link named. |
| `focus`       | Frames a region of the drawing that is already open.                       |
| `preferences` | The host's save preference: `autoSave` is `off`, `immediately` or `delayed`. |
| `save-request`| Asks the editor to write the drawing.                                      |
| `saved`       | The Project's text was written. Answers a `save`.                         |
| `save-failed` | The write failed. Answers a `save`.                                       |

Note application → figure frame:

| Message   | Meaning                                                                                  |
| --------- | ---------------------------------------------------------------------------------------- |
| `render`  | Renders a Project into a figure: `requestId`, `path`, `text`, and the `focus` region.  |

Editor frame → note application:

| Message | Meaning                                     |
| ------- | ------------------------------------------- |
| `ready` | The frame is mounted and answers messages.  |
| `dirty` | The drawing holds changes that are not on disk, or no longer does. |
| `save`  | Writes a Project back: `requestId`, `path`, `text`. |

Figure frame → note application:

| Message    | Meaning                                                          |
| ---------- | ---------------------------------------------------------------- |
| `ready`    | The frame answers render requests.                              |
| `rendered` | The figure: `requestId`, `path`, `svg`, `width`, `height`.      |
| `error`    | The request failed: `requestId`, `message`.                     |

A `save`, `render` and `rendered` exchange is bound by its `requestId`.

## Acceptance criteria

- A named region resolves to the bounds of its annotation, and a name no
  annotation carries resolves to nothing —
  [`region-target.test.ts`](../../apps/editor/src/features/circuit-region/region-target.test.ts).
- The editor's canvas fills the frame the note application gives it, and a link
  to a region frames that region instead of the whole sheet. Both are observed
  by the note application's end-to-end check.
- The drawing is written after the last edit when the host asks for that, the
  editor says when the drawing wants to be saved, and the host's save command
  reaches the drawing. All three are observed by the note application's
  end-to-end check.
- A note frame offers no second Project, and an unbound drawing is never
  written —
  [`file-command-menu.test.tsx`](../../apps/editor/src/features/editor-shell/file-command-menu.test.tsx),
  [`obsidian-project-store.test.ts`](../../apps/editor/src/hosts/obsidian-project-store.test.ts).
