import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { build } from "vite";
import { createHash } from "node:crypto";
import { expect, it } from "vitest";
import webConfig from "../vite.config";
import desktopConfig from "../vite.desktop.config";

const compositions = [
  ["Web", webConfig],
  ["desktop", desktopConfig],
] as const;

it.each(compositions)(
  "refuses a %s build with changed Canvas package bytes",
  async (_name, config) => {
    const directory = await mkdtemp(join(tmpdir(), "icm-rnl-package-"));
    try {
      const feature = join(directory, "src/features/netlist-export");
      await mkdir(feature, { recursive: true });
      await writeFile(join(feature, "analog-canvas-extensions.rnl"), "changed");
      const entry = join(directory, "entry.js");
      await writeFile(entry, "export const ready = true;");
      await expect(
        build({
          ...config,
          configFile: false,
          root: directory,
          logLevel: "silent",
          build: { write: false, rollupOptions: { input: entry } },
        }),
      ).rejects.toThrow(/Canvas package SHA-256 mismatch/u);
    } finally {
      if (!resolve(directory).startsWith(join(tmpdir(), "icm-rnl-package-")))
        throw new Error("Unexpected package test directory");
      await rm(directory, { recursive: true, force: true });
    }
  },
);

it.each(compositions)(
  "ships the locked raw bytes with the %s runtime",
  async (_name, config) => {
    const result = await build({
      ...config,
      configFile: false,
      root: resolve("apps/editor"),
      logLevel: "silent",
      build: {
        write: false,
        rollupOptions: {
          input: resolve(
            "apps/editor/src/features/netlist-export/rnl-runtime.ts",
          ),
        },
      },
    });
    if (Array.isArray(result) || !("output" in result))
      throw new Error("Expected one production bundle");
    const asset = result.output.find((item) =>
      /^assets\/analog-canvas-extensions-[^/]+\.rnl$/u.test(item.fileName),
    );
    if (!asset || asset.type !== "asset")
      throw new Error("Missing Canvas package asset");
    expect(createHash("sha256").update(asset.source).digest("hex")).toBe(
      "3738e1ce0c5d6fca3e13ad44faac8bd54f5eeb70577a22e412f5858f21f86711",
    );
  },
  60000,
);
