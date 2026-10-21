import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";
import {
  inverseTransformPoint,
  transformPoint,
  type CellSymbolPresentation,
  type CircuitProject,
  type Point,
  type SchematicDocument,
} from "@icm/model";
import {
  createBlockSymbol,
  hierarchicalBlockLabelWarnings,
  type SymbolDefinition,
  type SymbolResolver,
  type SymbolPin,
} from "@icm/symbols";
import {
  proposeBlockSymbolLayout,
  type BlockSymbolLayoutAction,
  type CellSymbolGrip,
} from "@icm/edit-engine";
import {
  selectedBlockSymbolTarget,
  type BlockSymbolLayoutTarget,
} from "./block-symbol-layout-target";
import type {
  CellSymbolLayoutInteraction,
  InteractionState,
} from "../../interaction/interaction-state";

type Instance = SchematicDocument["instances"][number];
type LayoutPointer = ReactPointerEvent<SVGElement>;
export interface CellSymbolLayoutSession {
  target: BlockSymbolLayoutTarget;
  instance: Instance;
  body: { left: number; right: number; top: number; bottom: number };
  pins: readonly {
    terminal: BlockSymbolLayoutTarget["terminals"][number];
    pin: SymbolPin;
  }[];
}
export interface CellSymbolLayoutPreview {
  symbol?: SymbolDefinition;
  reason?: string;
  marquee?: { start: Point; end: Point };
}

export function useCellSymbolLayout({
  project,
  document,
  selectedInstance,
  resolver,
  session,
  getSession,
  setSession,
  cancelInteraction,
  canvasPointFromEvent,
  save,
  report,
  startWire,
}: {
  project: CircuitProject;
  document: SchematicDocument;
  selectedInstance: Instance | undefined;
  resolver: SymbolResolver;
  session: CellSymbolLayoutInteraction | null;
  getSession: () => InteractionState<unknown>;
  setSession: (session: CellSymbolLayoutInteraction) => void;
  cancelInteraction: () => void;
  canvasPointFromEvent: (event: ReactPointerEvent<SVGSVGElement>) => Point;
  save: (
    target: BlockSymbolLayoutTarget,
    presentation: CellSymbolPresentation,
  ) => void;
  report: (message: string) => void;
  startWire: (instanceId: string, pinName: string) => void;
}) {
  const capture = useRef<{ element: SVGElement; pointerId: number } | null>(
    null,
  );
  const [actionPreview, setActionPreview] =
    useState<CellSymbolLayoutPreview | null>(null);
  const instance = document.instances.find(
    (item) => item.id === session?.instanceId,
  );
  const target = selectedBlockSymbolTarget(project, instance);
  const layout = useMemo<CellSymbolLayoutSession | null>(() => {
    if (
      !session ||
      session.documentId !== document.id ||
      !instance?.placement ||
      !target
    )
      return null;
    if (
      target.kind !== session.definitionTarget.kind ||
      target.ownerId !== session.definitionTarget.ownerId
    )
      return null;
    const definition = resolver.resolve(instance.symbolId)?.definition;
    const polygon = definition?.primitives.find(
      (item) => item.kind === "polygon",
    );
    if (!definition || polygon?.kind !== "polygon") return null;
    return {
      target,
      instance,
      body: {
        left: Math.min(...polygon.points.map((point) => point.x)),
        right: Math.max(...polygon.points.map((point) => point.x)),
        top: Math.min(...polygon.points.map((point) => point.y)),
        bottom: Math.max(...polygon.points.map((point) => point.y)),
      },
      pins: target.terminals.flatMap((terminal) => {
        const pin = definition.pins.find((item) => item.name === terminal.name);
        return pin ? [{ terminal, pin }] : [];
      }),
    };
  }, [document.id, instance, target, resolver, session?.instanceId]);
  const read = () => {
    const current = getSession();
    return current.kind === "cell-symbol-layout" ? current : null;
  };
  const release = () => {
    const held = capture.current;
    capture.current = null;
    if (held?.element.hasPointerCapture(held.pointerId))
      held.element.releasePointerCapture(held.pointerId);
  };
  const cancelDrag = () => {
    const current = read();
    if (current?.gesture) setSession({ ...current, gesture: null });
    release();
    setActionPreview(null);
  };
  const exit = () => {
    cancelInteraction();
    release();
    setActionPreview(null);
  };
  useEffect(() => {
    if (session && !layout) exit();
    else if (
      session?.gesture &&
      (target?.revision !== session.gesture.revision ||
        document.revision !== session.gesture.documentRevision ||
        project.structureRevision !== session.gesture.structureRevision)
    ) {
      cancelDrag();
      report(
        "Cell layout changed during the gesture; unfinished edit cancelled",
      );
    }
  }, [
    session?.documentId,
    session?.instanceId,
    target?.revision,
    document.revision,
    project.structureRevision,
    Boolean(layout),
  ]);
  useEffect(() => {
    if (!session?.gesture) release();
  }, [session?.gesture]);
  useEffect(() => {
    if (!session || !layout) return;
    setActionPreview(null);
    const selectedTerminalIds = session.selectedTerminalIds.filter((id) =>
      layout.pins.some(({ terminal }) => terminal.id === id),
    );
    if (selectedTerminalIds.length !== session.selectedTerminalIds.length)
      setSession({ ...session, selectedTerminalIds, gesture: null });
  }, [target?.revision]);
  const point = (event: LayoutPointer, local = true) => {
    const svg =
      event.currentTarget instanceof SVGSVGElement
        ? event.currentTarget
        : event.currentTarget.ownerSVGElement!;
    const world = canvasPointFromEvent({ ...event, currentTarget: svg });
    return local
      ? inverseTransformPoint(
          world,
          instance!.placement!.position,
          instance!.placement!,
        )
      : world;
  };
  const toggle = () => {
    if (read()) {
      exit();
      return;
    }
    const definition = selectedBlockSymbolTarget(project, selectedInstance);
    if (!selectedInstance?.placement || !definition) return;
    setSession({
      kind: "cell-symbol-layout",
      documentId: document.id,
      instanceId: selectedInstance.id,
      definitionTarget: { kind: definition.kind, ownerId: definition.ownerId },
      selectedTerminalIds: [],
      gesture: null,
    });
  };
  const beginDrag = (
    event: LayoutPointer,
    kind: "body" | "pin" | "marquee",
    id?: string,
  ) => {
    const current = read();
    if (!current || !layout || event.button !== 0 || event.altKey) return;
    event.preventDefault();
    event.stopPropagation();
    const additive = event.shiftKey || event.ctrlKey || event.metaKey;
    let selectedTerminalIds = current.selectedTerminalIds;
    if (kind === "pin" && id)
      selectedTerminalIds = additive
        ? selectedTerminalIds.includes(id)
          ? selectedTerminalIds.filter((item) => item !== id)
          : [...selectedTerminalIds, id]
        : selectedTerminalIds.includes(id)
          ? selectedTerminalIds
          : [id];
    setSession({
      ...current,
      selectedTerminalIds,
      gesture: {
        kind: kind === "body" ? "resize" : kind === "pin" ? "pins" : "marquee",
        pointerId: event.pointerId,
        start: point(event, kind !== "marquee"),
        clientStart: { x: event.clientX, y: event.clientY },
        revision: layout.target.revision,
        additive,
        documentRevision: document.revision,
        structureRevision: project.structureRevision,
        ...(kind === "body"
          ? { grip: id as CellSymbolGrip }
          : kind === "pin" && id
            ? { terminalId: id }
            : {}),
      },
    });
    capture.current = {
      element: event.currentTarget,
      pointerId: event.pointerId,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const moved = (event: LayoutPointer, current: CellSymbolLayoutInteraction) =>
    Math.hypot(
      event.clientX - current.gesture!.clientStart.x,
      event.clientY - current.gesture!.clientStart.y,
    ) >= 3;
  const actionAt = (event: LayoutPointer): BlockSymbolLayoutAction | null => {
    const current = read();
    const gesture = current?.gesture;
    if (
      !current ||
      !gesture ||
      !layout ||
      gesture.pointerId !== event.pointerId ||
      !moved(event, current)
    )
      return null;
    const local = point(event);
    if (gesture.kind === "resize")
      return {
        kind: "resize",
        grip: gesture.grip!,
        point: {
          x:
            local.x -
            gesture.start.x +
            (gesture.grip!.includes("west")
              ? layout.body.left
              : layout.body.right),
          y:
            local.y -
            gesture.start.y +
            (gesture.grip!.includes("north")
              ? layout.body.top
              : layout.body.bottom),
        },
      };
    if (gesture.kind !== "pins") return null;
    const selected = layout.pins.filter(({ terminal }) =>
      current.selectedTerminalIds.includes(terminal.id),
    );
    if (!selected.length) return null;
    let delta = { x: local.x - gesture.start.x, y: local.y - gesture.start.y };
    const sides = new Set(selected.map(({ pin }) => pin.direction));
    if (sides.size === 1) {
      const b = layout.body;
      const side = (
        [
          ["west", Math.abs(local.x - b.left)],
          ["east", Math.abs(local.x - b.right)],
          ["north", Math.abs(local.y - b.top)],
          ["south", Math.abs(local.y - b.bottom)],
        ] as const
      ).reduce((a, b) => (a[1] <= b[1] ? a : b))[0];
      const anchor = selected.find(
        ({ terminal }) => terminal.id === gesture.terminalId,
      )?.pin;
      if (!anchor) return null;
      const offset =
        anchor.direction === "west" || anchor.direction === "east"
          ? anchor.at.y
          : anchor.at.x;
      if (side !== anchor.direction)
        delta = { x: local.x - offset, y: local.y - offset };
      return {
        kind: "move-pins",
        terminalIds: current.selectedTerminalIds,
        delta,
        side,
      };
    }
    return {
      kind: "move-pins",
      terminalIds: current.selectedTerminalIds,
      delta,
    };
  };
  const previewAction = (
    action: BlockSymbolLayoutAction | null,
  ): CellSymbolLayoutPreview | null => {
    if (!layout || !action) return null;
    const proposal = proposeBlockSymbolLayout(layout.target, action);
    return proposal.ok
      ? {
          symbol: createBlockSymbol({
            ...layout.target,
            presentation: proposal.presentation,
          }),
          reason: hierarchicalBlockLabelWarnings(
            layout.target.terminals,
            proposal.presentation,
          ).join(" "),
        }
      : { reason: proposal.reason };
  };
  const previewDrag = (
    event: LayoutPointer,
  ): CellSymbolLayoutPreview | null => {
    const current = read();
    if (!current?.gesture || current.gesture.pointerId !== event.pointerId)
      return null;
    if (current.gesture.kind === "marquee")
      return {
        marquee: { start: current.gesture.start, end: point(event, false) },
      };
    return previewAction(actionAt(event));
  };
  const applyAction = (action: BlockSymbolLayoutAction) => {
    if (!layout) return;
    const proposal = proposeBlockSymbolLayout(layout.target, action);
    setActionPreview(null);
    if (proposal.ok) {
      if (proposal.changed) save(layout.target, proposal.presentation);
    } else report(proposal.reason);
  };
  const completeDrag = (event: ReactPointerEvent<SVGSVGElement>): boolean => {
    const current = read();
    const gesture = current?.gesture;
    if (
      !current ||
      !gesture ||
      gesture.pointerId !== event.pointerId ||
      !layout
    )
      return false;
    if (
      gesture.revision !== layout.target.revision ||
      gesture.documentRevision !== document.revision ||
      gesture.structureRevision !== project.structureRevision
    ) {
      cancelDrag();
      report("Cell layout changed during the gesture; nothing was changed");
      return true;
    }
    if (gesture.kind === "marquee") {
      const end = point(event, false),
        start = gesture.start;
      const crossing = event.clientX < gesture.clientStart.x;
      const ctm = event.currentTarget.getScreenCTM();
      const radius = 6 / (ctm ? Math.hypot(ctm.a, ctm.b) : 1);
      const ids = moved(event, current)
        ? layout.pins
            .filter(({ pin }) => {
              const local =
                pin.direction === "west"
                  ? { x: layout.body.left, y: pin.at.y }
                  : pin.direction === "east"
                    ? { x: layout.body.right, y: pin.at.y }
                    : pin.direction === "north"
                      ? { x: pin.at.x, y: layout.body.top }
                      : { x: pin.at.x, y: layout.body.bottom };
              const at = transformPoint(
                local,
                layout.instance.placement!.position,
                layout.instance.placement!,
              );
              const margin = crossing ? radius : -radius;
              return (
                at.x >= Math.min(start.x, end.x) - margin &&
                at.x <= Math.max(start.x, end.x) + margin &&
                at.y >= Math.min(start.y, end.y) - margin &&
                at.y <= Math.max(start.y, end.y) + margin
              );
            })
            .map(({ terminal }) => terminal.id)
        : [];
      setSession({
        ...current,
        gesture: null,
        selectedTerminalIds: gesture.additive
          ? [...new Set([...current.selectedTerminalIds, ...ids])]
          : ids,
      });
      release();
      return true;
    }
    const action = actionAt(event);
    cancelDrag();
    if (action) applyAction(action);
    else if (gesture.kind === "pins" && !gesture.additive && gesture.terminalId)
      setSession({
        ...current,
        gesture: null,
        selectedTerminalIds: [gesture.terminalId],
      });
    return true;
  };
  const handleKeyDown = (event: KeyboardEvent): boolean => {
    const current = read();
    if (!current || !layout) return false;
    if (event.key === "Escape") {
      if (current.gesture) cancelDrag();
      else exit();
      return true;
    }
    if (
      event.key.toLowerCase() === "w" &&
      !event.ctrlKey &&
      !event.metaKey &&
      !event.altKey
    ) {
      if (current.selectedTerminalIds.length !== 1) {
        report("Select exactly one layout pin to start Wire");
        return true;
      }
      const selected = layout.pins.find(
        ({ terminal }) => terminal.id === current.selectedTerminalIds[0],
      );
      if (!selected) {
        setSession({ ...current, selectedTerminalIds: [], gesture: null });
        report("Select exactly one current layout pin to start Wire");
        return true;
      }
      exit();
      startWire(layout.instance.id, selected.pin.name);
      return true;
    }
    const delta = (
      {
        ArrowLeft: { x: -10, y: 0 },
        ArrowRight: { x: 10, y: 0 },
        ArrowUp: { x: 0, y: -10 },
        ArrowDown: { x: 0, y: 10 },
      } as Record<string, Point>
    )[event.key];
    if (delta && !event.ctrlKey && !event.metaKey && !event.altKey) {
      applyAction({
        kind: "move-pins",
        terminalIds: current.selectedTerminalIds,
        delta: inverseTransformPoint(
          delta,
          { x: 0, y: 0 },
          layout.instance.placement!,
        ),
      });
      return true;
    }
    return false;
  };
  return {
    enabled: Boolean(session),
    layout,
    selectedTerminalIds: session?.selectedTerminalIds ?? [],
    activeDragPointerId: session?.gesture?.pointerId ?? null,
    warnings: layout
      ? hierarchicalBlockLabelWarnings(
          layout.target.terminals,
          layout.target.presentation,
        )
      : [],
    actionPreview,
    previewAction: (action: BlockSymbolLayoutAction | null) =>
      setActionPreview(previewAction(action)),
    applyAction,
    cancelDrag,
    exit,
    toggle,
    beginDrag,
    previewDrag,
    completeDrag,
    handleKeyDown,
  };
}
