import { createOfflineComposition } from "./build/offline-vite-config";

/**
 * Composition for the Go local server: the offline desktop renderer served
 * from http://127.0.0.1 with the browser-only entry adaptations.
 */
export default createOfflineComposition({
  entry: "/src/entries/local-web.tsx",
  outDir: "dist-local",
});
