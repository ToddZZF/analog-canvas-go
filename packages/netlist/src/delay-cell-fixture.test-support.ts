import {
  createEmptyDocument,
  createEmptyProject,
  type SchematicDocument,
} from "@icm/model";

function connect(
  document: SchematicDocument,
  name: string,
  instanceId: string,
  pinName: string,
) {
  let net = document.nets.find((net) => net.id === name);
  if (!net) {
    net = { id: name, terminals: [] };
    document.nets.push(net);
    document.connectivityEvidence.push({
      id: `name-${name}`,
      kind: "net-name-hint",
      netId: name,
      sourceName: name,
      origin: "spice-import",
    });
  }
  net.terminals.push({ instanceId, pinName });
}

/** Four-MOS educational Cell, not foundry or timing-qualified device data. */
export function delayCellFixture(transistor = false, load = "100f") {
  const project = createEmptyProject(
    "delay-proof",
    "Delay Cell acceptance",
    "dut",
  );
  const top = project.documents[0]!;
  top.netlist!.name = "dut";
  for (const [id, a, y] of transistor
    ? [["X1", "input", "output"]]
    : [
        ["X1", "input", "first"],
        ["X2", "first", "output"],
        ["X3", "pulse", "suppressed"],
      ]) {
    top.instances.push({
      id: id!,
      reference: id!,
      symbolId: "delay-cell",
      placement: {
        position: { x: 200 + top.instances.length * 150, y: 200 },
        rotation: 0,
        mirror: "none",
      },
      netlist: {
        binding: transistor
          ? { kind: "subcircuit", childDocumentId: "delay_cmos" }
          : { kind: "unresolved-subcircuit", name: "delay_cell" },
        parameters: transistor ? {} : { tpd: "100p", vt: "10m" },
      },
    });
    for (const [pin, name] of [
      ["A", a!],
      ["Y", y!],
      ["VDD", "VDD"],
      ["VSS", "reference"],
    ])
      connect(top, name!, id!, pin!);
  }
  if (!transistor) return project;
  const cell = createEmptyDocument("delay_cmos", "delay_cmos");
  cell.netlist!.name = "delay_cmos";
  for (const name of ["VDD", "VSS", "A", "Y"]) {
    cell.instances.push({
      id: `pin-${name}`,
      symbolId: "port",
      placement: null,
    });
    connect(cell, name, `pin-${name}`, "P");
    cell.netlist!.terminals.push({
      id: `terminal-${name}`,
      name,
      netId: name,
      direction: "inout",
      interfaceInstanceIds: [`pin-${name}`],
    });
  }
  for (const [id, kind, d, g, s] of [
    ["MP1", "pmos", "mid", "A", "VDD"],
    ["MN1", "nmos", "mid", "A", "VSS"],
    ["MP2", "pmos", "Y", "mid", "VDD"],
    ["MN2", "nmos", "Y", "mid", "VSS"],
  ] as const) {
    cell.instances.push({
      id,
      reference: id,
      symbolId: kind,
      placement: null,
      netlist: {
        binding: {
          kind: "model",
          deviceClass: "mos",
          name: kind === "nmos" ? "DN" : "DP",
        },
        parameters: { w: kind === "nmos" ? "1u" : "2u", l: "1u" },
      },
    });
    for (const [pin, name] of [
      ["D", d],
      ["G", g],
      ["S", s],
      ["B", s],
    ])
      connect(cell, name!, id, pin!);
  }
  for (const [id, node] of [
    ["C1", "mid"],
    ["C2", "Y"],
  ]) {
    cell.instances.push({
      id: id!,
      reference: id!,
      symbolId: "capacitor",
      placement: null,
      netlist: { parameters: { value: load } },
    });
    connect(cell, node!, id!, "1");
    connect(cell, "VSS", id!, "2");
  }
  project.documents.push(cell);
  return project;
}
