# Node selection

## Rules

`MinLoadNodeAssigner` picks node by:
1. status = active
2. capacity not exceeded
3. minimal current load

Protocol compatibility must be validated by control-plane before operation dispatch.

## Anti-drift note

Node load is not trusted purely as incremental counter; runtime reconciliation and apply reports are used to detect drifts.
