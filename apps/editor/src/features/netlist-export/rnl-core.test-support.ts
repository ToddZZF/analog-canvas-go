import { readFile } from "node:fs/promises";
import { startCore } from "@rich-netlist/runtime";
import { withCanvasPackage } from "./rnl-core";
import { decodeCanvasPackage } from "./rnl-canvas-package";

/** @internal The same verified local asset that the editor ships. */
export const canvasPackageText = readFile(
  new URL("./analog-canvas-extensions.rnl", import.meta.url),
).then(decodeCanvasPackage);

/** @internal Real Node core with the editor's package workspace, shared by contract tests. */
export async function createRnlTestCore() {
  const packageText = await canvasPackageText;
  return withCanvasPackage(startCore(), packageText);
}
