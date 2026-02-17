# interband

Shared sideband protocol library for Interverse modules.

## What it provides

- Standard envelope for sideband messages (`version`, `namespace`, `type`, `session_id`, `timestamp`, `payload`)
- Atomic writes (`tmp + rename`) to avoid partial-read races
- Centralized path helpers (default root: `~/.interband`)
- v1 payload reader with forward-compatible envelope checks

## Bash API

```bash
source lib/interband.sh

path=$(interband_path interphase bead "$CLAUDE_SESSION_ID")
interband_write "$path" interphase bead_phase "$CLAUDE_SESSION_ID" '{"id":"iv-123","phase":"executing","ts":123}'

interband_read_payload "$path"
```

## Versioning

Current protocol version: `1.0.0`.

Readers should accept `1.x` envelopes and ignore unknown payload fields.
