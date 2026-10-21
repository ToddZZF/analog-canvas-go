/**
 * Named circuit regions.
 *
 * A note may point at a part of a drawing instead of the whole sheet:
 * `[[inv.icproj.json|#Annotation]]` asks the canvas to open the project and
 * zoom to whatever `#Annotation` names.
 *
 * A Rectangle Annotation owns its name through the drafting text anchored to
 * it: the label the user typed inside the frame is the region's name. The
 * resolver therefore looks, in order, at
 *
 *   1. a Rectangle whose anchored label carries the name,
 *   2. a Rectangle's own stable id,
 *   3. a free drafting text that carries the name,
 *   4. an Instance reference (the `X1` on a part).
 *
 * Every candidate resolves to the same shape — a rectangle in document grid
 * units — so the caller only has to frame it.
 */
import { resolveDraftingObjectGeometry } from "@icm/derived";
import { flattenRichText, transformPoint } from "@icm/model";
import type {
  DerivedRect,
  DraftingObject,
  SchematicDocument,
} from "@icm/model";
import { resolveInstanceSymbol } from "@icm/symbols";
import type { SymbolResolver } from "@icm/symbols";

export type CircuitRegionKind =
  "rectangle-label" | "rectangle" | "text" | "instance";

export interface CircuitRegionTarget {
  /** What the name resolved to. */
  readonly kind: CircuitRegionKind;
  /** The id of the object that carries the name. */
  readonly objectId: string;
  /** The framed area, in document grid units. */
  readonly bounds: DerivedRect;
}

/** Names are compared without case or surrounding whitespace. */
function normalize(value: string): string {
  return value.trim().toLocaleLowerCase();
}

function rectangles(document: SchematicDocument): DraftingObject[] {
  return (document.drafting?.objects ?? []).filter(
    (object) => object.kind === "rectangle",
  );
}

/**
 * The drafting text anchored to a rectangle. The anchor resolves to the
 * rectangle's center, so the label follows the frame wherever it moves.
 */
function labelOf(
  document: SchematicDocument,
  rectangleId: string,
): Extract<DraftingObject, { kind: "text" }> | null {
  const label = (document.drafting?.objects ?? []).find(
    (candidate) =>
      candidate.kind === "text" &&
      candidate.anchor.kind === "object" &&
      candidate.anchor.objectId === rectangleId,
  );
  return label && label.kind === "text" ? label : null;
}

/**
 * Resolves a region name against one Document.
 *
 * Returns `null` when nothing carries that name; the caller decides how to
 * report it (the canvas falls back to framing the whole drawing).
 */
export function resolveCircuitRegion(
  document: SchematicDocument,
  resolver: SymbolResolver,
  name: string,
): CircuitRegionTarget | null {
  const wanted = normalize(name);
  if (wanted.length === 0) return null;

  // 1. A Rectangle Annotation named through its own label.
  for (const rectangle of rectangles(document)) {
    const label = labelOf(document, rectangle.id);
    if (!label) continue;
    if (normalize(flattenRichText(label.content)) !== wanted) continue;
    const geometry = resolveDraftingObjectGeometry(
      document,
      resolver,
      rectangle,
    );
    return {
      kind: "rectangle-label",
      objectId: rectangle.id,
      bounds: geometry.bounds,
    };
  }

  // 2. A Rectangle named by its stable id.
  for (const rectangle of rectangles(document)) {
    if (normalize(rectangle.id) !== wanted) continue;
    const geometry = resolveDraftingObjectGeometry(
      document,
      resolver,
      rectangle,
    );
    return {
      kind: "rectangle",
      objectId: rectangle.id,
      bounds: geometry.bounds,
    };
  }

  // 3. A free drafting text that carries the name.
  for (const object of document.drafting?.objects ?? []) {
    if (object.kind !== "text") continue;
    if (normalize(flattenRichText(object.content)) !== wanted) continue;
    const geometry = resolveDraftingObjectGeometry(document, resolver, object);
    return { kind: "text", objectId: object.id, bounds: geometry.bounds };
  }

  // 4. A placed part, named by the reference printed on it.
  for (const instance of document.instances) {
    const reference = instance.reference;
    if (!reference || normalize(reference) !== wanted) continue;
    if (!instance.placement) continue;
    const resolved = resolveInstanceSymbol(resolver, instance);
    if (!resolved) continue;
    const box = resolved.definition.viewBox;
    const corners = [
      { x: box.x, y: box.y },
      { x: box.x + box.width, y: box.y },
      { x: box.x + box.width, y: box.y + box.height },
      { x: box.x, y: box.y + box.height },
    ].map((point) =>
      transformPoint(point, instance.placement!.position, instance.placement!),
    );
    const xs = corners.map((point) => point.x);
    const ys = corners.map((point) => point.y);
    const left = Math.min(...xs);
    const top = Math.min(...ys);
    return {
      kind: "instance",
      objectId: instance.id,
      bounds: {
        x: left,
        y: top,
        width: Math.max(1, Math.max(...xs) - left),
        height: Math.max(1, Math.max(...ys) - top),
      },
    };
  }

  return null;
}

/**
 * Every name a Document offers, in the order the resolver prefers them.
 * The note app uses this to suggest completions after `#`.
 */
export function circuitRegionNames(document: SchematicDocument): string[] {
  const names = new Set<string>();
  for (const rectangle of rectangles(document)) {
    const label = labelOf(document, rectangle.id);
    if (!label) continue;
    const text = flattenRichText(label.content).trim();
    if (text.length > 0) names.add(text);
  }
  for (const object of document.drafting?.objects ?? []) {
    if (object.kind !== "text") continue;
    const text = flattenRichText(object.content).trim();
    if (text.length > 0) names.add(text);
  }
  for (const instance of document.instances) {
    const reference = instance.reference?.trim();
    if (reference) names.add(reference);
  }
  return [...names].sort((a, b) => a.localeCompare(b));
}
