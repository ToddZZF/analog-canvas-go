import type { BlockSymbolLayoutAction } from "@icm/edit-engine";

/** Transient contextual controls; never part of the formal Symbol scene. */
export function CellSymbolLayoutToolbar({
  name,
  selectedTerminalIds,
  warnings,
  onPreview,
  onApply,
  onDone,
}: {
  name: string;
  selectedTerminalIds: readonly string[];
  warnings: readonly string[];
  onPreview: (action: BlockSymbolLayoutAction | null) => void;
  onApply: (action: BlockSymbolLayoutAction) => void;
  onDone: () => void;
}) {
  const actionButton = (name: string, action: BlockSymbolLayoutAction) => (
    <button
      key={name}
      type="button"
      disabled={!selectedTerminalIds.length}
      onPointerEnter={() => onPreview(action)}
      onPointerLeave={() => onPreview(null)}
      onFocus={() => onPreview(action)}
      onBlur={() => onPreview(null)}
      onClick={() => onApply(action)}
    >
      {name}
    </button>
  );
  return (
    <div
      className="cell-symbol-layout-toolbar"
      role="toolbar"
      aria-label="Cell layout controls"
    >
      <span>
        <strong>{name}</strong> · all callers · {selectedTerminalIds.length}{" "}
        pins
      </span>
      {(["west", "north", "east", "south"] as const).map((side) =>
        actionButton(`Arrange ${side}`, {
          kind: "arrange-pins",
          side,
          terminalIds: selectedTerminalIds,
        }),
      )}
      {actionButton("Equal spacing", {
        kind: "space-pins",
        terminalIds: selectedTerminalIds,
      })}
      <button type="button" onClick={onDone}>
        Done
      </button>
      <small>
        Shift/Ctrl: select · arrows: 10 units · W: wire · Esc: cancel / done
      </small>
      {warnings.map((warning) => (
        <small key={warning}>{warning}</small>
      ))}
    </div>
  );
}
