import { createEmptyProject, type Instance } from "@icm/model";
import { withImportedInstanceDisplays } from "../src/features/instance-display/imported-instance-displays";

/** Synthetic interchange fixture; no user drawing is embedded here. */
export function bipolarProject() {
  const project = createEmptyProject("bipolar", "Bipolar interchange", "dut");
  const document = project.documents[0]!;
  document.netlist!.name = "dut";
  const at = (x: number, mirror: "none" | "horizontal" = "none") =>
    ({ position: { x, y: 200 }, rotation: 0, mirror }) as const;
  const devices: Instance[] = [
    {
      id: "Q1",
      reference: "Q1",
      symbolId: "pnp",
      placement: at(100, "horizontal"),
      netlist: {
        binding: { kind: "model", deviceClass: "bjt", name: "PNP" },
        parameters: { m: "2" },
      },
    },
    {
      id: "Q2",
      reference: "Q2",
      symbolId: "npn",
      placement: at(250),
      netlist: {
        binding: { kind: "model", deviceClass: "bjt", name: "NPN" },
        parameters: { m: "1" },
      },
    },
    {
      id: "D1",
      reference: "D1",
      symbolId: "diode",
      placement: at(400),
      netlist: {
        binding: { kind: "model", deviceClass: "diode", name: "DIODE" },
        parameters: {},
      },
    },
    {
      id: "M1",
      reference: "M1",
      symbolId: "nmos",
      placement: at(550, "horizontal"),
      netlist: {
        binding: { kind: "model", deviceClass: "mos", name: "NMOS" },
        parameters: { w: "1u", l: "1u" },
      },
    },
    { id: "P1", symbolId: "port", placement: at(700, "horizontal") },
  ];
  document.instances.push(...devices);
  const terminal = (instanceId: string, pinName: string) => ({
    instanceId,
    pinName,
  });
  document.nets.push(
    {
      id: "out",
      terminals: [
        terminal("Q1", "C"),
        terminal("Q2", "C"),
        terminal("D1", "A"),
        terminal("M1", "D"),
        terminal("P1", "P"),
      ],
    },
    {
      id: "bias",
      terminals: [
        terminal("Q1", "B"),
        terminal("Q2", "B"),
        terminal("M1", "G"),
      ],
    },
    {
      id: "tail",
      terminals: [
        terminal("Q1", "E"),
        terminal("Q2", "E"),
        terminal("D1", "K"),
        terminal("M1", "S"),
        terminal("M1", "B"),
      ],
    },
  );
  document.netlist!.terminals.push({
    id: "out-port",
    name: "out",
    netId: "out",
    direction: "inout",
    interfaceInstanceIds: ["P1"],
  });
  return withImportedInstanceDisplays(project);
}
