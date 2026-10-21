import type { EditorServices } from "./editor-services";
import { browserExportDelivery } from "../hosts/browser-export-delivery";
import { writeClipboardText } from "../hosts/clipboard-text";
import type { HostPersistence } from "../document/host-persistence";
import type { ZettlrProjectStore } from "../hosts/zettlr-project-store";

/**
 * The editor composition for a note application.
 *
 * A note frame owns exactly one Project, so everything that only makes sense
 * for a desktop shell — a Project library, a recent list, the community
 * gallery, the agent, the simulator — is left out. What remains is the
 * canvas, the parts library and the property inspector.
 */
export function createZettlrEditorServices(
  nativeProjectStore: ZettlrProjectStore,
  hostPersistence: HostPersistence,
): EditorServices {
  return {
    identity: null,
    projectStore: null,
    nativeProjectStore,
    capabilities: {
      community: false,
      localComponents: true,
      agent: false,
      simulation: false,
      externalLinks: false,
      // Every frame of a note application opens exactly one drawing, so no
      // frame ever brings tabs back.
      workspaceRestore: false,
      // The note application owns one drawing, and there is no second file
      // to put a new or an imported Project in.
      replaceDrawing: false,
      // The note application owns the file, so it decides when the drawing
      // is written.
      hostPersistence,
    },
    exportDelivery: {
      ...browserExportDelivery,
      copyText: writeClipboardText,
    },
  };
}
