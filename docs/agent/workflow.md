# Circuit changes and human handoff

Shared task policy; connection and request mechanics are in your transport's
entry guide.

## Evidence before editing

Identify the authorized Document, current revision and affected objects. Read
a complete Snapshot for unfamiliar topology, hierarchy changes or broad
rerouting; read placed instances' `pins` before wiring; read a known object's
geometry by stable ID. Counts are not pin or connectivity evidence.

Preserve topology unless the user asks for an electrical change, and preserve
human work outside the affected area, including locks and groups (a lock
conflict is not permission to unlock). Unresolved pin order, bulk/model
semantics or hierarchy binding needs an authoritative fact or a human answer.

## Bounded change

Use [native authoring](shared/authoring.md) and current revisions. Dry-run a
risky connectivity, destructive or multi-object edit when the risk warrants
it; MCP `apply_actions` already validates and returns a receipt, so add no
preview ritual. On a stale revision, refresh and reconsider intent; never
replay an outdated edit against a new revision. An uncertain transport result
is recovered with the original request identity. Interpret results with
[response semantics](response-semantics.md).

## Completion evidence

| Task | Evidence |
| --- | --- |
| Read/explain | Returned facts and remaining uncertainty |
| Local parameter change | Accepted change and current value; simulation only if requested |
| Placement, labels or routing | Changed geometry/connectivity and a formal render of the area |
| Whole schematic or hierarchy | Changed Documents and affected parent contexts reviewed |
| Drawing from a reference (figure, paper, netlist) | Done as [Drawing a schematic](circuit-style-knowledge.md) defines it: connected, compact, reads like the source |
| Simulation | Completed run, captured input identity and retrieved requested results |

For other visual changes use [Drawing a schematic](circuit-style-knowledge.md)
and [diagnostic policy](shared/diagnostics.md); do not clear observations
mechanically or polish unrelated areas. A render does not prove electrical
performance.

Report changed Documents, requested results and material unresolved issues
with object IDs. Leave a useful Document selected without changing the
Project's top Document. Never claim simulation correctness without naming the
simulator, model and analysis evidence.
