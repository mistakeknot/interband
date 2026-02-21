# interband

Shared sideband protocol library for Interverse modules. See `AGENTS.md` for philosophy alignment protocol.

## Quick Commands

```bash
# Source the library
source lib/interband.sh

# Write a sideband message
interband_write "$path" interphase bead_phase "$SESSION_ID" '{"id":"iv-123","phase":"executing"}'

# Read payload
interband_read_payload "$path"

# Prune stale sidebands
interband_prune_channel interphase bead
```

## Design Decisions (Do Not Re-Ask)

- Atomic writes via tmp + rename to prevent partial-read races
- Default root: `~/.interband`
- Protocol version: 1.0.0 — readers accept 1.x envelopes, ignore unknown fields
- Dual API: Bash (`lib/interband.sh`) and Go (`import "github.com/mistakeknot/interband"`)
- Retention defaults vary by channel (6h–24h, 128–256 files)
- interband = data sharing; interbase = code sharing (different concerns, same resolution pattern)
