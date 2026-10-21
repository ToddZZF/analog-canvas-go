import type {
  CellSymbolPinPlacement,
  CellSymbolPresentation,
} from "./schema/presentation.js";
import type { CellPortDirection } from "./cell-interface-projection.js";

type LayoutTerminal = {
  readonly id: string;
  readonly direction: CellPortDirection;
};

/** Pins leave one connection-grid step clear at each corner. */
export function cellSymbolPinFitsBody(
  pin: Pick<CellSymbolPinPlacement, "side" | "offset">,
  bounds: NonNullable<CellSymbolPresentation["bodyBounds"]>,
): boolean {
  const vertical = pin.side === "west" || pin.side === "east";
  return (
    pin.offset >= (vertical ? bounds.top : bounds.left) + 10 &&
    pin.offset <= (vertical ? bounds.bottom : bounds.right) - 10
  );
}

/** Resolve omitted placements once for geometry and authored-bounds validation. */
export function resolveCellSymbolPinPlacements(
  terminals: readonly LayoutTerminal[],
  presentation?: CellSymbolPresentation,
): CellSymbolPinPlacement[] {
  const stored = presentation?.pinPlacements ?? [];
  const explicit = new Map(stored.map((pin) => [pin.terminalId, pin]));
  const occupied = new Set(stored.map((pin) => `${pin.side}:${pin.offset}`));
  let westCount = stored.filter((pin) => pin.side === "west").length;
  let eastCount = stored.filter((pin) => pin.side === "east").length;
  return terminals.map((terminal) => {
    const pin = explicit.get(terminal.id);
    if (pin) return { ...pin };
    const side =
      terminal.direction === "input"
        ? "west"
        : terminal.direction === "output"
          ? "east"
          : westCount <= eastCount
            ? "west"
            : "east";
    let offset = 0;
    for (let step = 1; occupied.has(`${side}:${offset}`); step++)
      offset = (step % 2 ? -1 : 1) * Math.ceil(step / 2) * 20;
    occupied.add(`${side}:${offset}`);
    if (side === "west") westCount++;
    else eastCount++;
    return { terminalId: terminal.id, side, offset };
  });
}

/** Explicit edges contain automatic and authored pins alike. */
export function cellSymbolPinContainmentProblems(
  terminals: readonly LayoutTerminal[],
  presentation?: CellSymbolPresentation,
): string[] {
  const bounds = presentation?.bodyBounds;
  if (!bounds) return [];
  return resolveCellSymbolPinPlacements(terminals, presentation).flatMap(
    (pin) => {
      return cellSymbolPinFitsBody(pin, bounds)
        ? []
        : [
            `Cell symbol pin ${pin.terminalId} must stay inside its edge, 10 units from a corner`,
          ];
    },
  );
}
