import { createOfflineComposition } from "./build/offline-vite-config";

/** Separate composition entry/output: Web routes, telemetry and SW never boot. */
export default createOfflineComposition({
  entry: "/src/entries/desktop.tsx",
  outDir: "dist-desktop",
});
