# Node API contracts

Versioned headers for control-plane <-> node-agent transport.

- Header: `X-Protocol-Version`
- Current: `1`

Compatibility policy:
- control-plane accepts only known versions,
- node-agent always sends current version.
