# Drawing a schematic

For drawing or redrawing a schematic from a figure, paper or netlist, and for
reviewing how one looks. Tool calls and edit mechanics are in
[authoring](shared/authoring.md); this page says what a good drawing is.

## Done means three things

1. **Connected right.**
   - Before placing, write the reference as structural SPICE: MOS in `D G S B` order, the
     named ports, and a name for every node the source names.
   - After drawing, run `verify` with `expectedNetlist` and `details:true` until
     `topology: "equal"`.
   - `port-order` and `binding` differences can be harmless. Skip them with
     `compare:{portOrder:false,bindings:false}`, or name them in the report.
2. **Compact.** It meets the targets in [Compact layout](#compact-layout).
3. **Reads like the source.** The render passes [Review the render](#review-the-render).

A drawing that only matches its netlist is not done.

## Source first

- The figure decides placement, orientation, signal flow, names and printed text. The
  netlist decides connectivity. When they disagree, follow the netlist: keep the figure's
  arrangement and draw the extra parts beside it.
- Measure the figure in device pitches and keep its proportions at one device pitch = 60
  grid units. Never stretch it to make room for wires.
- A block the figure draws as a symbol (op-amp, comparator) but the netlist gives as
  transistors is drawn either way:
  - as a Cell placed where the symbol is;
  - flat, beside the main drawing, joined by Net labels.
- With no figure (netlist only): main signal path left to right, positive rail above, ground
  or the negative rail below, repeated sequences in tap order, and a visible supply name.

## Compact layout

- **Pitch** (a MOS or BJT symbol is 40 tall):

  | Between | Spacing |
  | --- | --- |
  | Parts stacked on one node | 60 (a 20-unit S–D wire) |
  | Parts side by side in a row | 60–80 |
  | Differential-pair halves | 80–120 |
  | Stage to stage | 100–140 |
  | Ports, Net labels, names | 20–40 from their pin |
  | Ground | 30 below its terminal |
  | Supply rail | 30–40 above the top row |

- **Supplies:** one rail fed by one marker or Port, unless the source draws a marker per part.
- **Long wires:** feedback and long returns run 30–40 outside the outermost part, never
  around an empty frame.
- **Unshown circuitry:** parts the source does not show (bias, a block's transistors) sit
  80–120 from the drawing, packed the same way.
- **Self-check** (targets from human-drawn Gallery circuits): area per part ≤ 10,000 grid²
  and nearest neighbour ≤ 55. Area per part is the bounding box of the part origins, plus 80
  on each axis, divided by the parts; Cells count separately. Also avoid a strip longer than
  3.5 times its width.
- **Compacting a loose drawing:** move whole areas with one selection `transform` so their
  internal wires come along. Redraw the few wires between areas instead of leaving
  stretched loops. If it is badly spread, redrawing is usually faster.

## Structures

Textbook defaults, chosen from actual pins and Nets. They are visual defaults, not
electrical evidence. Apply only the matching topology, and never rewire to fit a row.

| Structure | Layout | Wiring |
| --- | --- | --- |
| Differential pair | Left/right symmetry; corresponding pins aligned | Shared tail: T-join |
| Current mirror | Adjacent branches; reference gate faces inward toward output gates | Common-gate line where clear; reference local closure |
| Common-source stage | Gate toward input; drain toward output | Direct signal path |
| Common-gate stage | Stacked along its branch | Branch connection; bias from the side |
| Tail current source | Below an NMOS input pair | Centred drop to the shared tail |
| Compensation network | Beside the stages it spans | Separate return corridor |

| Wiring shape | Use |
| --- | --- |
| Direct | Short Port leads; matching inputs get equal leads; no detour through another group |
| T-join | Local shared node, e.g. short tail crossbar with centred drop |
| Trunk + taps | A clear aligned pin row for common control; a separate spine only when needed |
| Local closure | Short reference/diode connection beside its owner |
| Separate return | Feedback/compensation corridor apart from the forward path |
| Body tie | Dashed body wire by `via`: a stub out of the channel, then the shortest path to the target Net or a Junction on it |

Plan branched Nets whole: spine, taps, exits; branches get Junctions. Bound Net labels may
replace distant wires.

## Names and text

The editor typesets names itself: `M1` → M₁, `VB1` → V_B1, `vin` → v_in, `φ1` → φ₁.

- **Device references:** the source's names (M1, C2, Q3), within the
  [Reference rules](shared/authoring.md#place-and-name). Parts it does not name continue the
  numbering. Never use generated or netlist names such as `MM3_2`, `RRF` or `net1005`; they
  typeset wrongly.
- **Ports and Net labels:**
  - the source's text, as plain text ([labels](shared/authoring.md#labels-and-text));
  - drop function arguments such as `(t)`;
  - a Net the source leaves unnamed but that needs a label gets a short name (`VB1`, `VX`).
- **Equations and notes** printed beside the circuit: annotations with a math run, near the
  same spot.
- **Values and W/L:** in a new drawing, show them only when the source does. In a user's own
  drawing, keep default attached labels, manual positions, formatting and values unless
  asked; never erase values.

## Review the render

Render and look at it before reporting:
- no overlapping parts or labels;
- no wire through a symbol or an unrelated pin;
- names typeset like the source;
- the source's arrangement is recognisable at a glance;
- nothing is parked far away.

Fix what you see, at most two rounds.
