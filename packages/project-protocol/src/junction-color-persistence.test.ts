import { createEmptyProject } from "@icm/model";
import { describe, expect, it } from "vitest";
import { parseProject, serializeProject } from "./index.js";

function coloredProject() {
  const project = createEmptyProject("junction-color", "Junction color");
  project.documents[0]!.nets.push({ id: "net", terminals: [] });
  project.documents[0]!.junctions.push({
    id: "J",
    netId: "net",
    position: { x: 0, y: 0 },
    styleOverride: { color: "#CC2244" },
  });
  return project;
}

describe("junction color persistence", () => {
  it("round-trips a color without resolving Auto into saved metadata", () => {
    const project = coloredProject();
    const saved = serializeProject(project);
    expect(parseProject(saved)).toEqual(project);
    expect(serializeProject(parseProject(saved))).toBe(saved);
  });
  it("rejects junction colors in older file envelopes", () => {
    const source = JSON.parse(serializeProject(coloredProject()));
    source.schemaVersion = 72;
    expect(() => parseProject(JSON.stringify(source))).toThrow(
      /Junction colors require Project schema 73/u,
    );
  });
});
