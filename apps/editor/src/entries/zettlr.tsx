/**
 * The circuit editor inside a note application.
 *
 * The note application owns the file: it hands the Project over and it
 * writes the Project back. The editor owns the drawing.
 */
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { parseProject } from "@icm/project-protocol";
import { builtInSymbols, createProjectSymbolResolver } from "@icm/symbols";

import { App } from "../app/App";
import { EditorErrorBoundary } from "../components/editor-error-boundary";
import { createZettlrEditorServices } from "../services/zettlr-editor-services";
import { createZettlrProjectStore } from "../hosts/zettlr-project-store";
import type { ZettlrProjectStore } from "../hosts/zettlr-project-store";
import {
  onHostMessage,
  postToHost,
  type ZettlrAutoSaveMode,
  type ZettlrOpenFile,
} from "../hosts/zettlr-bridge";
import { resolveCircuitRegion } from "../features/circuit-region/region-target";
import type { HostPersistence } from "../document/host-persistence";
// The global sheet carries the design tokens and the shell's height chain
// (body, #root, .app-shell). Without it the editor has no definite height to
// lay its canvas out in, and the drawing is scaled to a canvas that overflows
// the note's frame.
import "../styles.css";

const root = document.getElementById("root");
if (!root) throw new Error("Editor root element is missing");

/** The Project the note application handed over. */
let handOver: (file: ZettlrOpenFile) => void = () => {};
const opened = new Promise<ZettlrOpenFile>((resolve) => {
  handOver = resolve;
});

/** The region a link pointed at, once the Project is known. */
let pendingFocus: string | undefined;
/** The drawing that is currently open, so a later region can be resolved. */
let currentFile: ZettlrOpenFile | undefined;
/** The retry timer of a region that arrived before the canvas was ready. */
let focusRetry: ReturnType<typeof setTimeout> | undefined;
/** How often a region may wait for the canvas before it is given up on. */
const FOCUS_RETRY_LIMIT = 100;
/** How often a region has waited for the canvas so far. */
let focusRetries = 0;

/** How eagerly the note application writes the drawing. */
let autoSaveMode: ZettlrAutoSaveMode = "off";

onHostMessage((message) => {
  if (message.type === "open" || message.type === "render") {
    pendingFocus = message.focus;
    const file: ZettlrOpenFile = {
      path: message.path,
      text: message.text,
      ...(message.focus ? { focus: message.focus } : {}),
    };
    currentFile = file;
    handOver(file);
    return;
  }
  if (message.type === "focus") {
    pendingFocus = message.region;
    applyFocus();
    return;
  }
  if (message.type === "preferences") {
    // The note application writes the drawing like it writes a note: as
    // eagerly as its own preference says.
    autoSaveMode = message.autoSave;
    return;
  }
  if (message.type === "save-request") {
    applySave();
  }
});

/**
 * Writes the drawing, the way the person pressing Save does.
 *
 * The note application owns the file, so its own save command reaches the
 * editor here. The editor may not be mounted yet, so the request waits for
 * it rather than being dropped.
 */
function applySave(): void {
  const host = window as unknown as {
    __analogCanvasSave?: () => Promise<unknown>;
  };
  if (!host.__analogCanvasSave) {
    scheduleSaveRetry();
    return;
  }
  void host.__analogCanvasSave();
}

/** The retry timer of a save that arrived before the editor was mounted. */
let saveRetry: ReturnType<typeof setTimeout> | undefined;
let saveRetries = 0;

/**
 * Waits for the editor before writing the drawing.
 */
function scheduleSaveRetry(): void {
  if (saveRetry !== undefined) return;
  if (saveRetries >= FOCUS_RETRY_LIMIT) return;
  saveRetries += 1;
  saveRetry = setTimeout(() => {
    saveRetry = undefined;
    applySave();
  }, 60);
}

/**
 * Frames the region a link pointed at.
 *
 * The camera belongs to the canvas, so the frame is handed over as a plain
 * rectangle in document units; the canvas decides how much of the drawing
 * around it stays visible.
 *
 * A region may arrive before the canvas exists — the note application hands
 * the drawing over and names the region in the same breath — so the request
 * waits for the canvas rather than being dropped.
 */
function applyFocus(): void {
  if (!pendingFocus) return;
  const host = window as unknown as {
    __analogCanvasFocusRegion?: (bounds: {
      x: number;
      y: number;
      width: number;
      height: number;
    }) => void;
  };
  if (!host.__analogCanvasFocusRegion) {
    scheduleFocusRetry();
    return;
  }
  const bounds = resolveRegionBounds(pendingFocus);
  if (!bounds) return;
  focusRetries = 0;
  host.__analogCanvasFocusRegion(bounds);
}

/**
 * Waits for the canvas to appear before framing a region.
 */
function scheduleFocusRetry(): void {
  if (focusRetry !== undefined) return;
  if (focusRetries >= FOCUS_RETRY_LIMIT) return;
  focusRetries += 1;
  focusRetry = setTimeout(() => {
    focusRetry = undefined;
    applyFocus();
  }, 60);
}

/**
 * The rectangle the named region covers, in document units.
 */
function resolveRegionBounds(name: string): {
  x: number;
  y: number;
  width: number;
  height: number;
} | null {
  const file = currentFile;
  if (!file) return null;
  try {
    const project = parseProject(file.text);
    const document =
      project.documents.find((item) => item.id === project.topDocumentId) ??
      project.documents[0];
    if (!document) return null;
    const resolver = createProjectSymbolResolver(project, builtInSymbols);
    const target = resolveCircuitRegion(document, resolver, name);
    return target?.bounds ?? null;
  } catch {
    // A drawing that cannot be parsed is reported by the editor itself; the
    // link simply lands on the whole document.
    return null;
  }
}

/** Writes the Project back through the note application. */
function writeThroughHost(file: ZettlrOpenFile, text: string): Promise<void> {
  const requestId = `save-${Date.now()}-${Math.random().toString(36).slice(2)}`;
  return new Promise<void>((resolve, reject) => {
    const stop = onHostMessage((message) => {
      if (message.type === "saved" && message.requestId === requestId) {
        stop();
        resolve();
        return;
      }
      if (message.type === "save-failed" && message.requestId === requestId) {
        stop();
        reject(new Error(message.message));
      }
    });
    postToHost({
      channel: "analog-canvas",
      type: "save",
      requestId,
      path: file.path,
      text,
    });
  });
}

const baseStore = createZettlrProjectStore({
  file: opened,
  write: (file, text) => writeThroughHost(file, text),
});

const store: ZettlrProjectStore = {
  ...baseStore,
  async open(recentId) {
    const outcome = await baseStore.open(recentId);
    if (outcome.status === "opened") {
      applyFocus();
    }
    return outcome;
  },
};

/**
 * How the note application writes the drawing.
 *
 * The note application keeps notes on disk without a save command, so the
 * drawing follows its preference; the editor only reports when the drawing
 * holds changes that are not on disk.
 */
const hostPersistence: HostPersistence = {
  autoSave: () => autoSaveMode,
  onDirtyChange: (dirty) => {
    postToHost({ channel: "analog-canvas", type: "dirty", dirty });
  },
};

createRoot(root).render(
  <StrictMode>
    <EditorErrorBoundary>
      <App services={createZettlrEditorServices(store, hostPersistence)} />
    </EditorErrorBoundary>
  </StrictMode>,
);

postToHost({ channel: "analog-canvas", type: "ready" });
