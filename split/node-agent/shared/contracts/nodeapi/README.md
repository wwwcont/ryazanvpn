# Node API contracts

Versioned headers for control-plane <-> node-agent transport.

- Header: `X-Protocol-Version`
- Current: `1`
- Previous (supported during rollout): `0`

Compatibility policy:
- control-plane accepts `N` and `N-1` protocol versions during cutover,
- node-agent always sends current version.
