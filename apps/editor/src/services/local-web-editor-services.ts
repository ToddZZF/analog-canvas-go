import type { EditorServices } from "./editor-services";
import { browserExportDelivery } from "../hosts/browser-export-delivery";
import { createLocalWebProjectStore } from "../hosts/local-web-project-store";
import { createPreviewEditorServices } from "./preview-editor-services";

/**
 * The Go local server composition: the offline desktop renderer with two
 * browser adaptations — exports download through the page, and opening a
 * project file uses the browser picker. The host API (/desktop/*) and all
 * library UI stay identical to the desktop shell.
 */
export function createLocalWebEditorServices(): EditorServices {
  const services = createPreviewEditorServices();
  return {
    ...services,
    nativeProjectStore: createLocalWebProjectStore(),
    exportDelivery: browserExportDelivery,
  };
}
