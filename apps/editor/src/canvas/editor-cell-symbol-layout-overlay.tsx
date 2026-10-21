import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";
import {
  transformPoint,
  type GridRect,
  type Point,
  type SchematicDocument,
} from "@icm/model";
import type { CellSymbolGrip } from "@icm/edit-engine";
import type { SymbolPin } from "@icm/symbols";
import type { CellSymbolLayoutPreview } from "../features/hierarchy/use-cell-symbol-layout";
import { ComponentPlacementPreview } from "../features/component-insert/component-placement-preview";

type Placement = NonNullable<
  SchematicDocument["instances"][number]["placement"]
>;
export function EditorCellSymbolLayoutOverlay({
  placement,
  body,
  pins,
  viewBounds,
  selectedTerminalIds = [],
  activeDragPointerId,
  actionPreview,
  onDragStart,
  onDragPreview,
  onDragCancel,
}: {
  placement: Placement;
  body: { left: number; right: number; top: number; bottom: number };
  pins: readonly { terminalId: string; pin: SymbolPin }[];
  viewBounds?: GridRect;
  selectedTerminalIds?: readonly string[];
  activeDragPointerId?: number | null;
  actionPreview?: CellSymbolLayoutPreview | null;
  onDragPreview?: (
    event: ReactPointerEvent<SVGGElement>,
  ) => CellSymbolLayoutPreview | null;
  onDragCancel?: () => void;
  onDragStart: (
    event: ReactPointerEvent<SVGElement>,
    kind: "body" | "pin" | "marquee",
    id?: string,
  ) => void;
}) {
  const root = useRef<SVGGElement | null>(null);
  const [unitsPerPixel, setUnitsPerPixel] = useState(1);
  const [preview, setPreview] = useState<CellSymbolLayoutPreview | null>(null);
  const pending = useRef<CellSymbolLayoutPreview | null>(null);
  const frame = useRef<number | null>(null);
  const clearPreview = () => {
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    pending.current = null;
    setPreview(null);
  };
  useEffect(() => {
    if (activeDragPointerId === null) clearPreview();
  }, [activeDragPointerId]);
  useEffect(
    () => () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
    },
    [],
  );
  useLayoutEffect(() => {
    const update = () => {
      const matrix = root.current?.getScreenCTM();
      if (matrix) setUnitsPerPixel(1 / Math.hypot(matrix.a, matrix.b));
    };
    update();
    const observer = new ResizeObserver(update);
    const camera = new MutationObserver(update);
    if (root.current?.ownerSVGElement) {
      observer.observe(root.current.ownerSVGElement);
      camera.observe(root.current.ownerSVGElement, {
        attributes: true,
        attributeFilter: ["viewBox"],
      });
    }
    return () => {
      observer.disconnect();
      camera.disconnect();
    };
  }, [viewBounds]);
  const shown = preview ?? actionPreview;
  const symbol = shown?.symbol;
  const previewBody = symbol?.primitives.find(
    (item) => item.kind === "polygon",
  );
  if (previewBody?.kind === "polygon")
    body = {
      left: Math.min(...previewBody.points.map((point) => point.x)),
      right: Math.max(...previewBody.points.map((point) => point.x)),
      top: Math.min(...previewBody.points.map((point) => point.y)),
      bottom: Math.max(...previewBody.points.map((point) => point.y)),
    };
  if (symbol)
    pins = pins.map(({ terminalId, pin }) => ({
      terminalId,
      pin: symbol.pins.find((item) => item.name === pin.name) ?? pin,
    }));
  const world = (point: Point) =>
    transformPoint(point, placement.position, placement);
  const midX = (body.left + body.right) / 2,
    midY = (body.top + body.bottom) / 2;
  const outside = 15 * unitsPerPixel;
  const grips: readonly [CellSymbolGrip, Point][] = [
    ["north-west", { x: body.left, y: body.top }],
    ["north-east", { x: body.right, y: body.top }],
    ["south-west", { x: body.left, y: body.bottom }],
    ["south-east", { x: body.right, y: body.bottom }],
    ["west", { x: body.left - outside, y: midY }],
    ["east", { x: body.right + outside, y: midY }],
    ["north", { x: midX, y: body.top - outside }],
    ["south", { x: midX, y: body.bottom + outside }],
  ];
  const marquee = shown?.marquee;
  return (
    <g
      ref={root}
      className="cell-symbol-layout-overlay"
      data-testid="cell-symbol-layout-overlay"
      onPointerMove={(event) => {
        const next = onDragPreview?.(event);
        if (!next) return;
        event.stopPropagation();
        pending.current = next;
        if (frame.current === null)
          frame.current = requestAnimationFrame(() => {
            frame.current = null;
            setPreview(pending.current);
          });
      }}
      onPointerUp={clearPreview}
      onPointerCancel={() => {
        clearPreview();
        onDragCancel?.();
      }}
      onLostPointerCapture={() => {
        clearPreview();
        onDragCancel?.();
      }}
    >
      {viewBounds ? (
        <rect
          data-testid="cell-symbol-layout-background"
          {...viewBounds}
          fill="transparent"
          style={{ pointerEvents: "all" }}
          onPointerDown={(event) => onDragStart(event, "marquee")}
        />
      ) : null}
      {symbol ? (
        <g
          className="cell-symbol-layout-preview"
          data-testid="cell-symbol-layout-preview"
        >
          <ComponentPlacementPreview
            symbol={symbol}
            symbolId={symbol.id}
            styleProfileId="razavi-textbook"
            position={placement.position}
            rotation={placement.rotation}
            mirror={placement.mirror}
          />
        </g>
      ) : null}
      {grips.map(([grip, local]) => {
        const at = world(local),
          centre = world({ x: midX, y: midY });
        const cursors = [
          "ew-resize",
          "nwse-resize",
          "ns-resize",
          "nesw-resize",
        ];
        const angle = Math.round(
          Math.atan2(at.y - centre.y, at.x - centre.x) / (Math.PI / 4),
        );
        return (
          <circle
            key={grip}
            data-testid={
              grip === "south-east"
                ? "cell-symbol-body-handle"
                : `cell-symbol-body-handle-${grip}`
            }
            aria-label={`Resize ${grip}`}
            className="cell-symbol-layout-handle body"
            cx={at.x}
            cy={at.y}
            r={7 * unitsPerPixel}
            vectorEffect="non-scaling-stroke"
            style={{ cursor: cursors[(angle + 8) % 4] }}
            onPointerDown={(event) => onDragStart(event, "body", grip)}
          />
        );
      })}
      {pins.map(({ terminalId, pin }) => {
        const local =
          pin.direction === "west"
            ? { x: body.left, y: pin.at.y }
            : pin.direction === "east"
              ? { x: body.right, y: pin.at.y }
              : pin.direction === "north"
                ? { x: pin.at.x, y: body.top }
                : { x: pin.at.x, y: body.bottom };
        const at = world(local),
          selected = selectedTerminalIds.includes(terminalId);
        return (
          <circle
            key={terminalId}
            data-testid={`cell-symbol-pin-handle-${terminalId}`}
            data-selected={selected}
            aria-label={`Layout pin ${pin.name}`}
            className={`cell-symbol-layout-handle pin${selected ? " selected" : ""}`}
            cx={at.x}
            cy={at.y}
            r={6 * unitsPerPixel}
            vectorEffect="non-scaling-stroke"
            onPointerDown={(event) => onDragStart(event, "pin", terminalId)}
          />
        );
      })}
      {marquee ? (
        <rect
          className="cell-symbol-layout-marquee"
          x={Math.min(marquee.start.x, marquee.end.x)}
          y={Math.min(marquee.start.y, marquee.end.y)}
          width={Math.abs(marquee.end.x - marquee.start.x)}
          height={Math.abs(marquee.end.y - marquee.start.y)}
          vectorEffect="non-scaling-stroke"
        />
      ) : null}
      {shown?.reason ? (
        <g
          transform={`translate(${world({ x: body.left, y: body.bottom }).x} ${world({ x: body.left, y: body.bottom }).y + 28 * unitsPerPixel}) scale(${unitsPerPixel})`}
        >
          <text
            data-testid="cell-symbol-layout-limit"
            className="cell-symbol-layout-limit"
            fontSize="12"
          >
            {shown.reason}
          </text>
        </g>
      ) : null}
    </g>
  );
}
