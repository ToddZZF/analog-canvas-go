import { routeEndpoints, type SchematicDocument } from "@icm/model";
import type { SymbolResolver } from "@icm/symbols";
import {
  deriveDocumentContactEvidence,
  type CoincidentContact,
} from "./contact.js";

/** One visible contact has one style owner, independent of its incident Wires. */
export function junctionForContact(
  contact: CoincidentContact,
  junctions: ReadonlyMap<string, SchematicDocument["junctions"][number]>,
): SchematicDocument["junctions"][number] | undefined {
  const candidates = contact.endpoints.flatMap((endpoint) => {
    const junction =
      endpoint.kind === "junction"
        ? junctions.get(endpoint.junctionId)
        : undefined;
    return junction ? [junction] : [];
  });
  return (
    candidates.find((junction) => junction.styleOverride?.color) ??
    candidates[0]
  );
}

/** A styled contact marker follows its pins when every pin at the contact travels. */
export function carriedContactJunctionIds(
  document: SchematicDocument,
  resolver: SymbolResolver,
  instanceIds: ReadonlySet<string>,
): string[] {
  if (instanceIds.size === 0) return [];
  const referenced = new Set(
    document.routes.flatMap((route) =>
      routeEndpoints(route).flatMap((e) =>
        e.kind === "junction" ? [e.junctionId] : [],
      ),
    ),
  );
  const markers = new Set(
    document.junctions
      .filter((j) => j.styleOverride?.color && !referenced.has(j.id))
      .map((j) => j.id),
  );
  if (markers.size === 0) return [];
  return deriveDocumentContactEvidence(document, resolver).contacts.flatMap(
    (contact) => {
      const terminals = contact.endpoints.filter((e) => e.kind === "terminal");
      if (
        terminals.length === 0 ||
        !terminals.every((e) => instanceIds.has(e.instanceId))
      )
        return [];
      return contact.endpoints.flatMap((e) =>
        e.kind === "junction" && markers.has(e.junctionId)
          ? [e.junctionId]
          : [],
      );
    },
  );
}
