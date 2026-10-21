import { useEffect, useState } from "react";
import type { WireSource } from "@icm/edit-engine";
import type { SchematicDocument } from "@icm/model";
import { endpointKey } from "@icm/derived";
import { nearestTerminal } from "../../canvas/terminal-pick-snap";

type Terminal = Extract<WireSource["endpoint"], { kind: "terminal" }>;

/** Transient picking only; both entry points commit the existing NoConnect edit. */
export function useNoConnectPlacement({
  active,
  document,
  viewBox,
  endpoints,
  mark,
  report,
}: {
  active: boolean;
  document: SchematicDocument;
  viewBox: { x: number; y: number; width: number; height: number };
  endpoints: readonly WireSource[];
  mark: (endpoint: Terminal, preserveInteraction: boolean) => void;
  report: (message: string) => void;
}) {
  const [hover, setHover] = useState<{
    key: string;
    unit: number;
    view: string;
  } | null>(null);
  useEffect(() => setHover(null), [active, document.id]);
  const view = `${viewBox.x} ${viewBox.y} ${viewBox.width} ${viewBox.height}`;
  // The live SVG camera settles into React later. Keep a hover already picked
  // against that new camera rather than clearing it on the delayed commit.
  useEffect(
    () => setHover((current) => (current?.view === view ? current : null)),
    [view],
  );
  // Resolve identity again after edits, undo or deletion: never paint old geometry.
  const source =
    active && hover
      ? endpoints.find(
          (candidate) => endpointKey(candidate.endpoint) === hover.key,
        )
      : undefined;
  const endpoint =
    source?.endpoint.kind === "terminal" ? source.endpoint : null;
  const marked =
    endpoint &&
    document.noConnects.some(
      (item) => endpointKey(item.endpoint) === endpointKey(endpoint),
    );
  const label = endpoint
    ? `${document.instances.find((item) => item.id === endpoint.instanceId)?.reference ?? endpoint.instanceId}.${endpoint.pinName}`
    : "";
  const state = marked ? "marked" : source?.netId ? "connected" : "available";
  const targetAt = (point: { x: number; y: number }, canvas: SVGSVGElement) => {
    const matrix = canvas.getScreenCTM();
    if (!matrix) return null;
    const source = nearestTerminal(
      endpoints.filter((candidate) => candidate.endpoint.kind === "terminal"),
      point,
      ({ connection: { contactPoint: at } }) => ({
        x: matrix.a * at.x + matrix.c * at.y + matrix.e,
        y: matrix.b * at.x + matrix.d * at.y + matrix.f,
      }),
      12,
    );
    return source
      ? {
          source,
          unit: 1 / Math.hypot(matrix.a, matrix.b),
          view: canvas.getAttribute("viewBox") ?? view,
        }
      : null;
  };
  return {
    active,
    preview:
      source && endpoint && hover
        ? {
            point: source.connection.contactPoint,
            unit: hover.unit,
            label,
            state,
          }
        : null,
    hoverAt(point: { x: number; y: number }, canvas: SVGSVGElement) {
      const target = targetAt(point, canvas);
      const key = target ? endpointKey(target.source.endpoint) : null;
      setHover((current) =>
        current?.key === key &&
        current?.unit === target?.unit &&
        current?.view === target?.view
          ? current
          : target
            ? { key: key!, unit: target.unit, view: target.view }
            : null,
      );
      if (target?.source.endpoint.kind === "terminal") {
        const pin = target.source.endpoint;
        const name = `${document.instances.find((item) => item.id === pin.instanceId)?.reference ?? pin.instanceId}.${pin.pinName}`;
        report(
          target.source.netId
            ? `${name}: connected — disconnect it before marking No Connect`
            : `${name}: No Connect — click to mark · Esc exits`,
        );
      } else
        report(
          "No Connect: move near a pin · overlapping pins are not guessed · Esc exits",
        );
    },
    placeAt(point: { x: number; y: number }, canvas: SVGSVGElement) {
      const target = targetAt(point, canvas);
      if (target?.source.endpoint.kind === "terminal")
        mark(target.source.endpoint, true);
      else
        report("No Connect: no unambiguous pin here · move closer to one pin");
    },
    clearHover: () => setHover(null),
  };
}

export function NoConnectPlacementPreview({
  target,
}: {
  target: ReturnType<typeof useNoConnectPlacement>["preview"];
}) {
  if (!target) return null;
  const { point, unit, label, state } = target;
  const color =
    state === "connected" ? "var(--icm-danger, #dc2626)" : "var(--icm-accent)";
  return (
    <g
      data-testid="no-connect-target"
      data-terminal={label}
      data-state={state}
      transform={`translate(${point.x} ${point.y}) scale(${unit})`}
      pointerEvents="none"
    >
      <circle r="7" fill="none" stroke={color} strokeWidth="1.5" />
      <path
        d="M -3 -3 L 3 3 M 3 -3 L -3 3"
        fill="none"
        stroke={color}
        strokeWidth="1.5"
      />
      <text x="12" y="-10" fontSize="12" fill={color}>
        {label} —{" "}
        {state === "connected"
          ? "Connected"
          : state === "marked"
            ? "Already NoConn"
            : "NoConn"}
      </text>
    </g>
  );
}
