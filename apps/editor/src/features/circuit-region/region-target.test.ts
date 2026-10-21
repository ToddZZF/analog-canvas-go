import { describe, expect, it } from "vitest";

import { builtInSymbols, createProjectSymbolResolver } from "@icm/symbols";
import type { CircuitProject, DraftingObject } from "@icm/model";
import { parseProject } from "@icm/project-protocol";
import { renderDocumentSvg } from "@icm/render-svg";

import twoStageOpAmp from "../../examples/two-stage-op-amp.icproj.json";

import { circuitRegionNames, resolveCircuitRegion } from "./region-target";

function project(): CircuitProject {
  return parseProject(JSON.stringify(twoStageOpAmp));
}

/** A rectangle with the label anchored to it, the way the editor writes one. */
function rectangleWithLabel(
  id: string,
  label: string,
  labelId: string,
): DraftingObject[] {
  return [
    {
      id,
      kind: "rectangle",
      locked: false,
      zIndex: 0,
      anchor: { kind: "free", position: { x: 0, y: 0 } },
      center: { x: 300, y: 200 },
      width: 200,
      height: 120,
      rotation: 0,
      lineStyle: "solid",
    },
    {
      id: labelId,
      kind: "text",
      locked: false,
      zIndex: 0,
      anchor: {
        kind: "object",
        objectId: id,
        localOffset: { x: 0, y: 0 },
        fallbackPosition: { x: 300, y: 200 },
      },
      content: { runs: [{ kind: "text", value: label }] },
      alignment: "middle",
      rotation: 0,
      typographyToken: "label",
    },
  ];
}

describe("circuit region resolution", () => {
  it("resolves a Rectangle Annotation by the name on its label", () => {
    const parsed = project();
    const document = parsed.documents[0]!;
    document.drafting = {
      objects: rectangleWithLabel("rect-1", "Annotation", "label-1"),
    };
    const resolver = createProjectSymbolResolver(parsed, builtInSymbols);

    const target = resolveCircuitRegion(document, resolver, "Annotation");
    expect(target).not.toBeNull();
    expect(target!.kind).toBe("rectangle-label");
    expect(target!.objectId).toBe("rect-1");
    expect(target!.bounds.width).toBeGreaterThan(0);
    expect(target!.bounds.height).toBeGreaterThan(0);
  });

  it("matches a region name without regard to case or padding", () => {
    const parsed = project();
    const document = parsed.documents[0]!;
    document.drafting = {
      objects: rectangleWithLabel("rect-1", "Bias Block", "label-1"),
    };
    const resolver = createProjectSymbolResolver(parsed, builtInSymbols);

    expect(
      resolveCircuitRegion(document, resolver, "  bias block "),
    ).not.toBeNull();
  });

  it("resolves a Rectangle by its stable id", () => {
    const parsed = project();
    const document = parsed.documents[0]!;
    document.drafting = {
      objects: rectangleWithLabel("rect-9", "Anything", "label-1"),
    };
    const resolver = createProjectSymbolResolver(parsed, builtInSymbols);

    const target = resolveCircuitRegion(document, resolver, "rect-9");
    expect(target?.kind).toBe("rectangle");
    expect(target?.objectId).toBe("rect-9");
  });

  it("resolves a part by its reference", () => {
    const parsed = project();
    const document = parsed.documents[0]!;
    const resolver = createProjectSymbolResolver(parsed, builtInSymbols);
    const instance = document.instances.find((item) => item.reference);
    expect(instance?.reference).toBeTruthy();

    const target = resolveCircuitRegion(
      document,
      resolver,
      instance!.reference!,
    );
    expect(target?.kind).toBe("instance");
  });

  it("reports no region for an unknown name", () => {
    const parsed = project();
    const resolver = createProjectSymbolResolver(parsed, builtInSymbols);
    expect(
      resolveCircuitRegion(parsed.documents[0]!, resolver, "nothing-here"),
    ).toBeNull();
  });

  it("offers the names a drawing carries", () => {
    const parsed = project();
    const document = parsed.documents[0]!;
    document.drafting = {
      objects: rectangleWithLabel("rect-1", "Annotation", "label-1"),
    };
    expect(circuitRegionNames(document)).toContain("Annotation");
  });

  it("cuts the figure down to the region a note points at", () => {
    const parsed = project();
    const document = parsed.documents[0]!;
    document.drafting = {
      objects: rectangleWithLabel("rect-1", "Annotation", "label-1"),
    };
    const resolver = createProjectSymbolResolver(parsed, builtInSymbols);
    const target = resolveCircuitRegion(document, resolver, "Annotation");
    expect(target).not.toBeNull();

    const whole = renderDocumentSvg(document, resolver);
    const cropped = renderDocumentSvg(document, resolver, {
      bounds: {
        x: Math.floor(target!.bounds.x),
        y: Math.floor(target!.bounds.y),
        width: Math.max(1, Math.ceil(target!.bounds.width)),
        height: Math.max(1, Math.ceil(target!.bounds.height)),
      },
    });

    const viewBox = (svg: string): number[] =>
      (svg.match(/viewBox="([^"]+)"/u)?.[1] ?? "")
        .split(" ")
        .map((value) => Number.parseFloat(value));

    const wholeBox = viewBox(whole);
    const croppedBox = viewBox(cropped);
    expect(croppedBox[2]).toBeLessThan(wholeBox[2]!);
    expect(croppedBox[3]).toBeLessThan(wholeBox[3]!);
    expect(croppedBox[2]).toBeGreaterThan(0);
    expect(croppedBox[3]).toBeGreaterThan(0);
  });
});
