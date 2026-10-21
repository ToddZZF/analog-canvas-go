/**
 * Prepares a test workspace: a drawing with a named Rectangle Annotation,
 * and a note that links to it and shows it.
 *
 * Usage: node scripts/make-test-workspace.mjs <target-dir>
 */
import { mkdirSync, writeFileSync, readFileSync } from "node:fs";
import path from "node:path";

const target = process.argv[2];
if (!target) {
  console.error("Usage: node scripts/make-test-workspace.mjs <target-dir>");
  process.exit(1);
}

const source = path.resolve(
  import.meta.dirname,
  "../apps/editor/src/examples/two-stage-op-amp.icproj.json",
);
const project = JSON.parse(readFileSync(source, "utf8"));

const document = project.documents[0];
document.drafting = document.drafting ?? { objects: [] };

// A Rectangle Annotation that frames the output stage.
document.drafting.objects.push(
  {
    id: "rectangle-1",
    kind: "rectangle",
    locked: false,
    zIndex: 0,
    anchor: { kind: "free", position: { x: 430, y: 120 } },
    center: { x: 430, y: 120 },
    width: 260,
    height: 180,
    rotation: 0,
    lineStyle: "dashed",
  },
  {
    id: "note-1",
    kind: "text",
    locked: false,
    zIndex: 0,
    anchor: {
      kind: "object",
      objectId: "rectangle-1",
      localOffset: { x: 0, y: 0 },
      fallbackPosition: { x: 430, y: 120 },
    },
    content: { runs: [{ kind: "text", value: "OutputStage" }] },
    alignment: "middle",
    rotation: 0,
    typographyToken: "label",
  },
);

mkdirSync(target, { recursive: true });
writeFileSync(
  path.join(target, "inv.icproj.json"),
  JSON.stringify(project, null, 2),
  "utf8",
);
writeFileSync(
  path.join(target, "notes.md"),
  `# Circuit notes

An inverter drawing lives next to this note.

Open the whole drawing: [[inv.icproj.json]]

Open one region of it: [[inv.icproj.json|#OutputStage]]

Show the drawing here:

![[inv.icproj.json]]

Show one region here:

![[inv.icproj.json|#OutputStage]]
`,
  "utf8",
);
console.log(`Wrote test workspace to ${target}`);
