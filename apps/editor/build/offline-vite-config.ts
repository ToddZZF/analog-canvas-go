import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
import { isOnlineImplementation } from "./desktop-module-policy";

export interface OfflineComposition {
  entry: string;
  outDir: string;
}

/**
 * Shared composition for builds that never reach the hosted backend: the
 * desktop preview shell and the Go local server both serve this bundle.
 * Web routes, telemetry and the service worker never boot, and the
 * online-module guard hard-errors if any cloud implementation reaches the
 * output, so the offline promise stays enforced at build time.
 */
export function createOfflineComposition(composition: OfflineComposition) {
  return defineConfig({
    base: "/",
    // The RNL runtime Worker uses top-level await in the offline editor too.
    worker: { format: "es" },
    // Offline artifacts never inherit local .env files or public-prefixed
    // values. Hosted feature flags and credentials belong to the
    // Web/server composition.
    envDir: false,
    envPrefix: [],
    define: { "import.meta.env.ICM_DESKTOP": "true" },
    plugins: [
      react(),
      {
        name: "desktop-module-evidence",
        // Online leaf modules have no required offline initialization. Removing
        // their unused imports must not keep a lazy online UI alive as a side effect.
        transform(code, id) {
          if (isOnlineImplementation(id))
            return { code, moduleSideEffects: false };
        },
        generateBundle(_options, bundle) {
          const modules = new Set<string>();
          for (const item of Object.values(bundle)) {
            if (item.type === "chunk")
              for (const [id, module] of Object.entries(item.modules)) {
                if (module.renderedLength)
                  modules.add(
                    id
                      .replaceAll("\\", "/")
                      .replace(
                        resolve(import.meta.dirname, "../..").replaceAll(
                          "\\",
                          "/",
                        ) + "/",
                        "",
                      ),
                  );
                if (module.renderedLength && isOnlineImplementation(id))
                  this.error(`Online implementation in offline output: ${id}`);
              }
          }
          this.emitFile({
            type: "asset",
            fileName: "desktop-modules.json",
            source: JSON.stringify([...modules].sort(), null, 2),
          });
        },
      },
      {
        name: "offline-entry",
        transformIndexHtml: {
          order: "pre",
          handler: (html) =>
            html
              .replace("/src/main.tsx", composition.entry)
              .replace(/\s*<link rel="manifest"[^>]*>/u, ""),
        },
      },
    ],
    build: { outDir: composition.outDir },
  });
}
