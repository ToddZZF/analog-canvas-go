/**
 * The circuit figure inside a note.
 *
 * A note that embeds a drawing shows the drawing itself: the top sheet,
 * rendered as the same formal SVG the editor exports. A link that names a
 * region shows that region instead of the whole sheet.
 *
 * The frame never becomes part of the editor: it only reads Projects and
 * hands back figures.
 */
import { prepareDocumentFormulaArtifacts } from "@icm/derived";
import { parseProject } from "@icm/project-protocol";
import { renderDocumentSvg } from "@icm/render-svg";
import { builtInSymbols, createProjectSymbolResolver } from "@icm/symbols";

import { onHostMessage, postToHost } from "../hosts/obsidian-bridge";
import { resolveCircuitRegion } from "../features/circuit-region/region-target";

onHostMessage((message) => {
  if (message.type !== "render") return;
  void render(message.requestId, message.path, message.text, message.focus);
});

// The note application waits for this before it asks for a figure.
postToHost({ channel: "analog-canvas", type: "ready" });

async function render(
  requestId: string,
  path: string,
  text: string,
  focus?: string,
): Promise<void> {
  try {
    const project = parseProject(text);
    const document =
      project.documents.find((item) => item.id === project.topDocumentId) ??
      project.documents[0];
    if (!document) throw new Error("The drawing has no sheet");
    const resolver = createProjectSymbolResolver(project, builtInSymbols);
    const prepared = await prepareDocumentFormulaArtifacts(document);
    try {
      let crop: { x: number; y: number; width: number; height: number } | null =
        null;
      if (focus) {
        const target = resolveCircuitRegion(document, resolver, focus);
        if (target) {
          // The crop is a grid rectangle: the figure is cut on whole grid
          // lines, so nothing of the region is clipped away.
          const left = Math.floor(target.bounds.x);
          const top = Math.floor(target.bounds.y);
          crop = {
            x: left,
            y: top,
            width: Math.max(1, Math.ceil(target.bounds.width)),
            height: Math.max(1, Math.ceil(target.bounds.height)),
          };
        }
      }
      const svg = renderDocumentSvg(document, resolver, {
        title: document.name,
        ...(crop ? { bounds: crop } : {}),
      });
      const size = figureSize(svg);
      postToHost({
        channel: "analog-canvas",
        type: "rendered",
        requestId,
        path,
        svg,
        width: size.width,
        height: size.height,
      });
    } finally {
      prepared.release();
    }
  } catch (error) {
    postToHost({
      channel: "analog-canvas",
      type: "error",
      requestId,
      message: error instanceof Error ? error.message : String(error),
    });
  }
}

/** Reads the figure's own viewBox, so the note can size the image. */
function figureSize(svg: string): { width: number; height: number } {
  const match = /viewBox="([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)\s+([-\d.]+)"/u.exec(
    svg,
  );
  if (!match) return { width: 960, height: 640 };
  return {
    width: Number.parseFloat(match[3] ?? "960") || 960,
    height: Number.parseFloat(match[4] ?? "640") || 640,
  };
}
