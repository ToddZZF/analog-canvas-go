import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import type { Plugin } from "vite";
import { decodeCanvasPackage } from "../src/features/netlist-export/rnl-canvas-package";

/** Both compositions verify the vendored bytes before Vite copies the asset. */
export function lockedCanvasPackage(): Plugin {
  let path = "";
  return {
    name: "locked-canvas-package",
    configResolved(config) {
      path = resolve(
        config.root,
        "src/features/netlist-export/analog-canvas-extensions.rnl",
      );
    },
    async buildStart() {
      this.addWatchFile(path);
      await decodeCanvasPackage(await readFile(path));
    },
  };
}
