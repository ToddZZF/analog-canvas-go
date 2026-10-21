import type { SymbolDefinition } from "@icm/symbols";

/** Library icon only: placement creates a NoConnect declaration, never an Instance. */
export const noConnectPreviewSymbol = {
  schemaVersion: 1,
  id: "no-connect",
  name: "No Connect",
  pins: [],
  variants: [],
  labelVisibility: "hidden",
  decorative: true,
  viewBox: { x: -10, y: -10, width: 20, height: 20 },
  primitives: [
    { kind: "line", from: { x: -5, y: -5 }, to: { x: 5, y: 5 } },
    { kind: "line", from: { x: 5, y: -5 }, to: { x: -5, y: 5 } },
  ],
} satisfies SymbolDefinition;
