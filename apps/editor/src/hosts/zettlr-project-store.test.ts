import { describe, expect, it, vi } from "vitest";
import { createEmptyProject, createId } from "@icm/model";

import { createZettlrProjectStore } from "./zettlr-project-store";
import type { ZettlrOpenFile } from "./zettlr-bridge";

const handedOver: ZettlrOpenFile = {
  path: "/notes/inv.icproj.json",
  text: "{}",
};

const binding = {
  id: "zettlr-project",
  revision: 1,
  name: "inv.icproj.json",
  path: "/notes/inv.icproj.json",
};

const project = () =>
  createEmptyProject(createId("project"), "New Circuit", createId("document"));

describe("the note application's Project store", () => {
  it("writes the drawing the note application handed over", async () => {
    const write = vi.fn(async () => {});
    const store = createZettlrProjectStore({
      file: Promise.resolve(handedOver),
      write,
    });

    const outcome = await store.save(project(), binding);

    expect(outcome.status).toBe("saved");
    expect(write).toHaveBeenCalledTimes(1);
  });

  it("refuses to write a drawing that is bound to no file", async () => {
    const write = vi.fn(async () => {});
    const store = createZettlrProjectStore({
      file: Promise.resolve(handedOver),
      write,
    });

    // An editor that is no longer bound to a file has no drawing to write.
    // Writing the hand-over path anyway would destroy the drawing the note
    // still points at.
    const outcome = await store.save(project(), null);

    expect(outcome.status).toBe("failed");
    expect(write).not.toHaveBeenCalled();
  });
});
