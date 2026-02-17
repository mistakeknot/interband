# interband

Shared sideband protocol library for Interverse modules.

## What it provides

- Standard envelope for sideband messages (`version`, `namespace`, `type`, `session_id`, `timestamp`, `payload`)
- Atomic writes (`tmp + rename`) to avoid partial-read races
- Centralized path helpers (default root: `~/.interband`)
- Schema validation for known message contracts
- v1 readers with forward-compatible envelope checks

## Bash API

```bash
source lib/interband.sh

path=$(interband_path interphase bead "$CLAUDE_SESSION_ID")
interband_write "$path" interphase bead_phase "$CLAUDE_SESSION_ID" '{"id":"iv-123","phase":"executing","ts":123}'

interband_read_payload "$path"
interband_read_envelope "$path"
```

## Versioning

Current protocol version: `1.0.0`.

Readers should accept `1.x` envelopes and ignore unknown payload fields.

Known validated payload contracts:

- `interphase/bead_phase`: `id`, `phase`, `reason`, `ts`
- `clavain/dispatch`: `name`, `workdir`, `activity`, `started`, `turns`, `commands`, `messages`
- `interlock/coordination_signal`: `layer`, `icon`, `text`, `priority`, `ts`
