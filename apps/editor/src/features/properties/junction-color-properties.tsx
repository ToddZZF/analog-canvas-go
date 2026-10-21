import { z } from "zod";
import type { ItemPropertyIdentity } from "./item-property-code";
import type { PropertyResult } from "./annotation-property-code";
import { AnnotationPropertyCodeEditor } from "./annotation-property-code-editor";
import { annotationPropertyAdapter } from "./annotation-property-code-assists";
import { colorToRgb, parseCanvasColor } from "./component-property-fields";

const schema = z.strictObject({
  appearance: z.strictObject({ color: z.unknown() }),
});
function parse(source: string): PropertyResult<string> {
  try {
    const value = schema.parse(JSON.parse(source));
    return {
      ok: true,
      value: parseCanvasColor(value.appearance.color, "appearance.color"),
    };
  } catch (error) {
    return {
      ok: false,
      message:
        error instanceof Error ? error.message : "Invalid Junction color",
    };
  }
}
const adapter = annotationPropertyAdapter(parse, false, "Junction color");
const format = (color: string) =>
  JSON.stringify(
    { appearance: { color: color === "auto" ? "auto" : colorToRgb(color) } },
    null,
    2,
  );

export function JunctionColorProperties({
  item,
  color,
  inheritedColor,
  onApply,
}: {
  item: ItemPropertyIdentity;
  color: string;
  inheritedColor: string;
  onApply(color: string): { ok: boolean; message?: string };
}) {
  return (
    <>
      <AnnotationPropertyCodeEditor
        item={item}
        baseline={format(color)}
        adapter={adapter}
        parse={parse}
        format={format}
        onApply={onApply}
        defaultColor={inheritedColor}
        title="Junction"
      />
      <small>
        Auto follows incident wire colors; mixed colors use the Document
        foreground.
      </small>
    </>
  );
}
