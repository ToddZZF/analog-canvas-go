/**
 * The bridge between the circuit editor and the note application.
 *
 * The editor runs inside the note application's frame. The two talk through
 * `postMessage` on one channel, so the editor never needs file access of its
 * own: the note application hands over the Project text, and the editor hands
 * back the text it wants to keep.
 *
 * Every message carries the channel name, so a frame that hosts something
 * else entirely cannot be mistaken for this one.
 */
export const ZETTLR_CHANNEL = "analog-canvas";

/** How eagerly the note application writes the drawing. */
export type ZettlrAutoSaveMode = "off" | "immediately" | "delayed";

export interface ZettlrOpenFile {
  /** Absolute path of the `.icproj.json` file. */
  readonly path: string;
  /** The Project file's text. */
  readonly text: string;
  /** The region a link pointed at, if any. */
  readonly focus?: string;
}

/** What the note application tells the editor. */
export type NoteAppMessage =
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "open";
      path: string;
      text: string;
      focus?: string;
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "render";
      requestId: string;
      path: string;
      text: string;
      focus?: string;
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "focus";
      region: string;
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "preferences";
      autoSave: ZettlrAutoSaveMode;
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "save-request";
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "saved";
      requestId: string;
      path: string;
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "save-failed";
      requestId: string;
      message: string;
    };

/** What the editor tells the note application. */
export type EditorMessage =
  | { channel: typeof ZETTLR_CHANNEL; type: "ready" }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "dirty";
      dirty: boolean;
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "save";
      requestId: string;
      path: string;
      text: string;
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "rendered";
      requestId: string;
      path: string;
      svg: string;
      width: number;
      height: number;
    }
  | {
      channel: typeof ZETTLR_CHANNEL;
      type: "error";
      requestId?: string;
      message: string;
    };

const NOTE_APP_TYPES = new Set([
  "open",
  "render",
  "focus",
  "preferences",
  "save-request",
  "saved",
  "save-failed",
]);

function isNoteAppMessage(value: unknown): value is NoteAppMessage {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<NoteAppMessage>;
  if (candidate.channel !== ZETTLR_CHANNEL) return false;
  return (
    typeof candidate.type === "string" && NOTE_APP_TYPES.has(candidate.type)
  );
}

/** Posts a message to the note application that hosts this frame. */
export function postToHost(message: EditorMessage): void {
  if (window.parent === window) return;
  window.parent.postMessage(message, "*");
}

/**
 * Watches for the note application's messages.
 *
 * The listener stays installed for the life of the frame; the editor and the
 * renderer host both outlive every single request.
 */
export function onHostMessage(
  handler: (message: NoteAppMessage) => void,
): () => void {
  const listener = (event: MessageEvent): void => {
    if (event.source !== window.parent) return;
    if (!isNoteAppMessage(event.data)) return;
    handler(event.data);
  };
  window.addEventListener("message", listener);
  return () => window.removeEventListener("message", listener);
}
