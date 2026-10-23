import { describe, expect, it, vi } from "vitest";
import { createEmptyProject, createId } from "@icm/model";

import { createObsidianProjectStore } from "./obsidian-project-store";
import type { ObsidianOpenFile } from "./obsidian-bridge";

const handedOver: ObsidianOpenFile = {
  path: "/notes/inv.icproj.json",
  text: "{}",
};

/** The same drawing, renamed in the note application while it was open. */
const renamed: ObsidianOpenFile = {
  path: "/notes/inv-2024.icproj.json",
  text: "{}",
};

const binding = {
  id: "obsidian-project:/notes/inv.icproj.json",
  revision: 1,
  name: "inv.icproj.json",
  path: "/notes/inv.icproj.json",
};

const project = () =>
  createEmptyProject(createId("project"), "New Circuit", createId("document"));

describe("the note application's Project store", () => {
  it("writes the drawing the note application handed over", async () => {
    const write = vi.fn(async () => handedOver);
    const store = createObsidianProjectStore({
      file: () => handedOver,
      write,
    });

    const outcome = await store.save(project(), binding);

    expect(outcome.status).toBe("saved");
    expect(write).toHaveBeenCalledTimes(1);
  });

  it("refuses to write a drawing that is bound to no file", async () => {
    const write = vi.fn(async () => handedOver);
    const store = createObsidianProjectStore({
      file: () => handedOver,
      write,
    });

    // An editor that is no longer bound to a file has no drawing to write.
    // Writing the hand-over path anyway would destroy the drawing the note
    // still points at.
    const outcome = await store.save(project(), null);

    expect(outcome.status).toBe("failed");
    expect(write).not.toHaveBeenCalled();
  });

  it("opens the drawing the note application named last", async () => {
    let current: ObsidianOpenFile | undefined = handedOver;
    const store = createObsidianProjectStore({
      file: () => current,
      write: async () => handedOver,
    });

    // A pane that shows another drawing hands its own over again: the
    // editor follows the pane, not the first drawing it ever saw.
    current = renamed;
    const outcome = await store.open();

    expect(outcome.status).toBe("opened");
    if (outcome.status !== "opened") return;
    expect(outcome.file.path).toBe(renamed.path);
  });

  it("owes one launch per hand-over", async () => {
    let current: ObsidianOpenFile | undefined = handedOver;
    const store = createObsidianProjectStore({
      file: () => current,
      write: async () => handedOver,
    });

    expect(await store.nextLaunch?.()).not.toBeNull();
    const outcome = await store.open();
    expect(outcome.status).toBe("opened");
    // The drawing the editor was given is owed no launch again until the
    // note application hands another over.
    expect(await store.nextLaunch?.()).toBeNull();

    current = renamed;
    expect(await store.nextLaunch?.()).not.toBeNull();
  });

  it("keeps the drawing bound under the name the note application wrote", async () => {
    // A drawing that was renamed while it was open is written under its new
    // name, and the editor is bound under that name from then on.
    const write = vi.fn(async () => renamed);
    const store = createObsidianProjectStore({
      file: () => handedOver,
      write,
    });

    const outcome = await store.save(project(), binding);

    expect(outcome.status).toBe("saved");
    if (outcome.status !== "saved") return;
    expect(outcome.file.path).toBe(renamed.path);
    expect(outcome.file.name).toBe("inv-2024.icproj.json");
  });
});
