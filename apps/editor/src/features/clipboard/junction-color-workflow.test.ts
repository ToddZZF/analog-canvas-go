import { createEmptyProject, type SchematicDocument } from "@icm/model";
import { DocumentHistory, planRoutingTransform } from "@icm/edit-engine";
import { builtInSymbols, InMemorySymbolResolver } from "@icm/symbols";
import { parseProject, serializeProject } from "@icm/project-protocol";
import { buildSvgScene } from "@icm/render-svg";
import { resolveEndpointConnection } from "@icm/derived";
import { describe, expect, it } from "vitest";
import { captureProjectCopy } from "./project-copy";
import { proposePaste } from "./paste-proposal";
import { orientClipboard } from "./copy-placement";

const resolver = new InMemorySymbolResolver(builtInSymbols);
function request(document: SchematicDocument, edits: readonly unknown[]) {
  return {
    transactionId: `color-${document.revision}`,
    documentId: document.id,
    expectedRevision: document.revision,
    actor: { kind: "human", id: "test" },
    edits,
  };
}

describe("generated junction color workflow", () => {
  it.each([
    [0, 0],
    [45, 0],
    [0, 45],
    [45, 45],
  ] as const)(
    "keeps color and dot size through copy, move and reload at %i degrees plus %i copy rotation",
    (rotation, copyRotation) => {
      const project = createEmptyProject("source", "Source");
      const document = project.documents[0]!;
      for (const [index, id] of ["R1", "R2", "R3", "unselected"].entries())
        document.instances.push({
          id,
          symbolId: "resistor",
          documentStyle: { junctionRadiusScale: 2 },
          placement: {
            position: { x: index === 3 ? 200 : 0, y: 20 },
            rotation,
            mirror: "none",
          },
        });
      document.nets.push({
        id: "net",
        terminals: ["R1", "R2", "R3"].map((instanceId) => ({
          instanceId,
          pinName: "1",
        })),
      });
      const originalCircle = buildSvgScene(document, resolver).formalBody.match(
        /<circle[^>]*r="([^"]+)"[^>]*fill=/u,
      );
      expect(originalCircle).not.toBeNull();
      const history = new DocumentHistory(document, {
        symbolResolver: resolver,
      });
      expect(
        history.transact(
          request(document, [
            {
              kind: "set_junction_style_override",
              target: { kind: "terminal", instanceId: "R1", pinName: "1" },
              styleOverride: { color: "#CC2244" },
            },
          ]),
        ),
      ).toMatchObject({ ok: true });
      project.documents[0] = history.document;
      const single = captureProjectCopy(project, history.document, {
        instanceIds: ["R1"],
        routeIds: [],
        junctionIds: [],
        annotationIds: [],
        draftingIds: [],
      })!;
      expect(single.junctions).toEqual([]);
      const clipboard = captureProjectCopy(project, history.document, {
        instanceIds: ["R1", "R2", "R3"],
        routeIds: [],
        junctionIds: [],
        annotationIds: [],
        draftingIds: [],
      })!;
      expect(clipboard.junctions).toEqual([
        expect.objectContaining({ styleOverride: { color: "#CC2244" } }),
      ]);
      const target = createEmptyProject("target", "Target");
      const oriented = orientClipboard(
        clipboard,
        copyRotation ? [{ kind: "rotate", deltaDegrees: copyRotation }] : [],
        { x: 0, y: 20 },
        { resolver, presentation: target.documents[0]!.presentation },
      );
      const paste = proposePaste(
        target.documents[0]!,
        oriented,
        { x: 100, y: 100 },
        1,
        target,
      );
      const pasted = new DocumentHistory(target.documents[0]!, {
        symbolResolver: resolver,
      });
      expect(
        pasted.transact(request(pasted.document, paste.edits)),
      ).toMatchObject({ ok: true });
      expect(pasted.document.junctions).toEqual([
        expect.objectContaining({
          styleOverride: { color: "#CC2244" },
        }),
      ]);
      const plan = planRoutingTransform(
        pasted.document,
        resolver,
        {
          instanceIds: pasted.document.instances.map((i) => i.id),
          routeIds: [],
          junctionIds: [],
        },
        { kind: "translate", delta: { x: 40, y: 20 } },
      );
      expect(
        pasted.transact(request(pasted.document, plan.edits)),
      ).toMatchObject({ ok: true });
      target.documents[0] = pasted.document;
      const loaded = parseProject(serializeProject(target));
      const body = buildSvgScene(loaded.documents[0]!, resolver).formalBody;
      expect(body).toMatch(/<circle[^>]*fill="#CC2244"/u);
      const contact = resolveEndpointConnection(
        loaded.documents[0]!,
        resolver,
        {
          kind: "terminal",
          instanceId: pasted.document.instances[0]!.id,
          pinName: "1",
        },
      )!;
      const dot = body.match(
        /<circle[^>]*cx="([^"]+)" cy="([^"]+)"[^>]*fill="#CC2244"/u,
      )!;
      expect(Number(dot[1])).toBeCloseTo(contact.contactPoint.x, 4);
      expect(Number(dot[2])).toBeCloseTo(contact.contactPoint.y, 4);
      expect(body.match(/<circle[^>]*r="([^"]+)"[^>]*fill=/u)?.[1]).toBe(
        originalCircle?.[1],
      );
      expect(
        pasted.transact(request(pasted.document, [{ kind: "undo" }])),
      ).toMatchObject({ ok: true });
      expect(
        pasted.transact(request(pasted.document, [{ kind: "redo" }])),
      ).toMatchObject({ ok: true });
      expect(buildSvgScene(pasted.document, resolver).formalBody).toBe(body);
      expect(
        pasted.transact(
          request(pasted.document, [
            {
              kind: "set_junction_style_override",
              target: {
                kind: "terminal",
                instanceId: pasted.document.instances[0]!.id,
                pinName: "1",
              },
              styleOverride: null,
            },
          ]),
        ),
      ).toMatchObject({ ok: true });
      const autoBody = buildSvgScene(pasted.document, resolver).formalBody;
      expect(autoBody).not.toContain('fill="#CC2244"');
      expect(autoBody.match(/<circle[^>]*r="([^"]+)"[^>]*fill=/u)?.[1]).toBe(
        originalCircle?.[1],
      );
    },
  );
});
