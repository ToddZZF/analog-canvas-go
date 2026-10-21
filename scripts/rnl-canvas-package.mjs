import { readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  canvasPackageLock as lock,
  decodeCanvasPackage,
} from "../apps/editor/src/features/netlist-export/rnl-canvas-package.ts";

const asset = new URL(
  "../apps/editor/src/features/netlist-export/analog-canvas-extensions.rnl",
  import.meta.url,
);

/** Download only the catalog record pinned by name, version, URL and raw hash. */
export async function downloadCanvasPackage(request = fetch) {
  const response = await request(lock.catalogUrl);
  if (!response.ok) throw new Error(`Catalog HTTP ${response.status}`);
  const catalog = await response.json();
  if (catalog?.schemaVersion !== 1 || !Array.isArray(catalog.packages))
    throw new Error("Unsupported package catalog");
  const record = catalog.packages.find(
    (item) => item?.name === lock.name && item.version === lock.version,
  );
  if (
    !record ||
    record.sha256 !== lock.sha256 ||
    typeof record.url !== "string" ||
    new URL(record.url, lock.catalogUrl).href !== lock.url
  )
    throw new Error("Canvas package lock differs from catalog");
  const download = await request(lock.url);
  if (!download.ok) throw new Error(`Canvas package HTTP ${download.status}`);
  const bytes = new Uint8Array(await download.arrayBuffer());
  await decodeCanvasPackage(bytes);
  return bytes;
}

if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  try {
    if (process.argv.includes("--fetch")) {
      const bytes = await downloadCanvasPackage();
      await writeFile(asset, bytes);
    } else {
      await decodeCanvasPackage(await readFile(asset));
    }
    console.log(
      `Verified ${lock.name} ${lock.version}: SHA-256 ${lock.sha256}`,
    );
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
