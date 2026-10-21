import { describe, expect, it } from "vitest";
import { createEmptyProject } from "@icm/model";
import { instanceParameterContract } from "@icm/devices";
import { createDesignNetlistExport } from "./export.js";

function delayProject(parameters: Record<string, string> = {}) {
  const project = createEmptyProject("delay", "Delay Cell", "dut");
  const document = project.documents[0]!;
  document.instances.push({
    id: "delay",
    reference: "X1",
    symbolId: "delay-cell",
    placement: { position: { x: 0, y: 0 }, rotation: 0, mirror: "none" },
    netlist: { parameters },
  });
  for (const pinName of ["A", "Y"]) {
    document.nets.push({
      id: pinName,
      terminals: [{ instanceId: "delay", pinName }],
    });
    document.connectivityEvidence.push({
      id: `name-${pinName}`,
      kind: "net-name-hint",
      netId: pinName,
      sourceName: pinName,
      origin: "spice-import",
    });
  }
  return project;
}

describe("Delay Cell electrical model", () => {
  it("validates a pin-compatible Buffer explicitly calling the delay model", () => {
    const project = delayProject({ tpd: "-1n", vt: "10m" });
    const instance = project.documents[0]!.instances[0]!;
    instance.symbolId = "buffer";
    instance.netlist!.binding = {
      kind: "unresolved-subcircuit",
      name: "delay_cell",
    };
    const result = createDesignNetlistExport(project);
    expect(result.status).toBe("blocked");
    expect(result.diagnostics).toContainEqual(
      expect.objectContaining({ code: "INVALID_DELAY_CELL_PARAMETER" }),
    );
  });
  it("offers propagation parameters and exports a complete body without changing the drawing", () => {
    const project = delayProject({ tpd: "100p", vt: "10m" });
    const before = structuredClone(project);
    const contract = instanceParameterContract(
      project,
      project.documents[0]!.instances[0]!,
    );
    expect(contract?.definitions.map((p) => [p.name, p.defaultValue])).toEqual([
      ["vt", "10m"],
      ["tpd", "10p"],
    ]);
    const result = createDesignNetlistExport(project, { format: "spice" });
    if (result.status !== "ready")
      throw Error(JSON.stringify(result.diagnostics));
    expect(result.externalMasterCount).toBe(0);
    expect(result.file.text).toContain(
      ".subckt delay_cell VDD VSS A Y params: vt=10m tpd=10p",
    );
    expect(result.file.text).toMatch(
      /^X1 VDD VSS A Y delay_cell tpd=100p vt=10m$/mu,
    );
    expect(project).toEqual(before);
  });

  it.each([
    { tpd: "0" },
    { tpd: "-1n" },
    { vt: "0" },
    { vt: "-1m" },
    { tpd: "{period/2}" },
  ])(
    "rejects invalid formula parameters %j before simulation",
    (parameters) => {
      const result = createDesignNetlistExport(delayProject(parameters));
      expect(result.status).toBe("blocked");
      expect(result.diagnostics).toContainEqual(
        expect.objectContaining({ code: "INVALID_DELAY_CELL_PARAMETER" }),
      );
    },
  );
});
