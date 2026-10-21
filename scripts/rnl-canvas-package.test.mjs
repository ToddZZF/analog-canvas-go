import { expect, it, vi } from "vitest";
import { readFile } from "node:fs/promises";
import { downloadCanvasPackage } from "./rnl-canvas-package.mjs";

const record = {
  name: "analog-canvas-extensions",
  version: "0.1",
  url: "packages/analog-canvas-extensions/0.1/analog-canvas-extensions.rnl",
  sha256: "3738e1ce0c5d6fca3e13ad44faac8bd54f5eeb70577a22e412f5858f21f86711",
};
const bytes = await readFile(
  new URL(
    "../apps/editor/src/features/netlist-export/analog-canvas-extensions.rnl",
    import.meta.url,
  ),
);

it("rejects a changed catalog lock before downloading package bytes", async () => {
  const request = vi.fn().mockResolvedValue(
    Response.json({
      schemaVersion: 1,
      packages: [
        {
          name: "analog-canvas-extensions",
          version: "0.1",
          url: "packages/analog-canvas-extensions/0.1/analog-canvas-extensions.rnl",
          sha256: "0".repeat(64),
        },
      ],
    }),
  );
  await expect(downloadCanvasPackage(request)).rejects.toThrow(
    /Canvas package lock differs from catalog/u,
  );
  expect(request).toHaveBeenCalledTimes(1);
});

it("downloads the locked version even when the catalog adds newer versions", async () => {
  const request = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json({
        schemaVersion: 1,
        packages: [
          { ...record, version: "0.2", sha256: "new-version" },
          record,
        ],
      }),
    )
    .mockResolvedValueOnce(new Response(bytes));
  expect(await downloadCanvasPackage(request)).toEqual(new Uint8Array(bytes));
  expect(request.mock.calls.map(([url]) => url)).toEqual([
    "https://rich-netlist.github.io/rnl-packages/catalog.json",
    "https://rich-netlist.github.io/rnl-packages/packages/analog-canvas-extensions/0.1/analog-canvas-extensions.rnl",
  ]);
});

it("rejects changed raw bytes even when the catalog hash still matches", async () => {
  const request = vi
    .fn()
    .mockResolvedValueOnce(
      Response.json({ schemaVersion: 1, packages: [record] }),
    )
    .mockResolvedValueOnce(
      new Response(Buffer.concat([bytes, Buffer.from("\n")])),
    );
  await expect(downloadCanvasPackage(request)).rejects.toThrow(
    /Canvas package SHA-256 mismatch/u,
  );
});

it.each([2, "1", null])("rejects catalog schema %s", async (schemaVersion) => {
  const request = vi
    .fn()
    .mockResolvedValue(Response.json({ schemaVersion, packages: [record] }));
  await expect(downloadCanvasPackage(request)).rejects.toThrow(
    /Unsupported package catalog/u,
  );
});

it.each([
  ["missing version", [{ ...record, version: "0.2" }]],
  ["changed URL", [{ ...record, url: "https://example.test/changed.rnl" }]],
])("rejects a %s in the catalog", async (_name, packages) => {
  const request = vi
    .fn()
    .mockResolvedValue(Response.json({ schemaVersion: 1, packages }));
  await expect(downloadCanvasPackage(request)).rejects.toThrow(
    /Canvas package lock differs from catalog/u,
  );
});

it.each(["Catalog", "Canvas package"])(
  "reports %s HTTP failures",
  async (stage) => {
    const request = vi.fn();
    if (stage === "Canvas package")
      request.mockResolvedValueOnce(
        Response.json({ schemaVersion: 1, packages: [record] }),
      );
    request.mockResolvedValueOnce(new Response("unavailable", { status: 503 }));
    await expect(downloadCanvasPackage(request)).rejects.toThrow(
      `${stage} HTTP 503`,
    );
  },
);
