import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";

import { isOnlineImplementation } from "./build/desktop-module-policy";
import { lockedCanvasPackage } from "./build/rnl-canvas-package";

/**
 * The bundle a note application embeds.
 *
 * Two frames come out of it:
 *
 *   * `obsidian.html` — the editor, for opening and editing a drawing inside
 *     a note,
 *   * `render.html` — the figure, for showing a drawing inside a note.
 *
 * The bundle never reaches the hosted backend: the note application is the
 * only host, and it hands Projects over frame by frame. The online-module
 * guard below keeps that promise true at build time.
 */
export default defineConfig({
  base: "/",
  // The RNL runtime Worker uses top-level await in the offline editor too.
  worker: { format: "es" },
  envDir: false,
  envPrefix: [],
  define: { "import.meta.env.ICM_DESKTOP": "true" },
  plugins: [
    lockedCanvasPackage(),
    react(),
    {
      name: "desktop-module-evidence",
      transform(code, id) {
        if (isOnlineImplementation(id))
          return { code, moduleSideEffects: false };
      },
      generateBundle(_options, bundle) {
        for (const item of Object.values(bundle)) {
          if (item.type !== "chunk") continue;
          for (const id of Object.keys(item.modules)) {
            if (!item.modules[id]?.renderedLength) continue;
            if (isOnlineImplementation(id))
              this.error(`Online implementation in the note bundle: ${id}`);
          }
        }
      },
    },
  ],
  build: {
    outDir: "dist-obsidian",
    rollupOptions: {
      input: {
        obsidian: resolve(import.meta.dirname, "obsidian.html"),
        render: resolve(import.meta.dirname, "render.html"),
      },
    },
  },
});
