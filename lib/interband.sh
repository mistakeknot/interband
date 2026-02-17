#!/usr/bin/env bash
# Shared sideband protocol helpers for Interverse modules.
#
# Contract:
# - Message envelope schema version 1.x
# - Atomic writes (tmp + rename)
# - Centralized default root: ~/.interband

[[ -n "${_INTERBAND_LOADED:-}" ]] && return 0
_INTERBAND_LOADED=1

interband_root() {
    echo "${INTERBAND_ROOT:-${HOME}/.interband}"
}

interband_protocol_version() {
    echo "${INTERBAND_PROTOCOL_VERSION:-1.0.0}"
}

interband_safe_key() {
    local raw="${1:-}"
    local safe
    safe=$(echo "$raw" | sed -e 's#[/[:space:]]#_#g' -e 's/[^A-Za-z0-9._-]/_/g')
    if [[ -z "$safe" ]]; then
        safe="default"
    fi
    echo "$safe"
}

interband_path() {
    local namespace="${1:-}" channel="${2:-}" key="${3:-}"
    [[ -n "$namespace" && -n "$channel" && -n "$key" ]] || return 1
    printf '%s/%s/%s/%s.json\n' \
        "$(interband_root)" \
        "$namespace" \
        "$channel" \
        "$(interband_safe_key "$key")"
}

interband_validate_payload() {
    local namespace="${1:-}" type="${2:-}" payload_json="${3:-}"
    [[ -n "$payload_json" ]] || return 1
    command -v jq >/dev/null 2>&1 || return 1

    # All payloads must be objects.
    echo "$payload_json" | jq -e 'type == "object"' >/dev/null 2>&1 || return 1

    # Unknown namespace/type pairs stay forward-compatible.
    case "${namespace}:${type}" in
        interphase:bead_phase)
            echo "$payload_json" | jq -e '
                (.id | type == "string" and length > 0) and
                (.phase | type == "string" and test("^(brainstorm|brainstorm-reviewed|strategized|planned|plan-reviewed|executing|shipping|done)$")) and
                ((.reason // "") | type == "string") and
                (.ts | type == "number")
            ' >/dev/null 2>&1 || return 1
            ;;
        clavain:dispatch)
            echo "$payload_json" | jq -e '
                (.name | type == "string" and length > 0) and
                (.workdir | type == "string" and length > 0) and
                (.activity | type == "string" and length > 0) and
                (.started | type == "number" and . >= 0) and
                (.turns | type == "number" and . >= 0) and
                (.commands | type == "number" and . >= 0) and
                (.messages | type == "number" and . >= 0)
            ' >/dev/null 2>&1 || return 1
            ;;
        interlock:coordination_signal)
            echo "$payload_json" | jq -e '
                (.layer | type == "string" and length > 0) and
                (.icon | type == "string" and length > 0) and
                (.text | type == "string" and length > 0) and
                (.priority | type == "number" and . >= 0) and
                (.ts | type == "string" and length > 0)
            ' >/dev/null 2>&1 || return 1
            ;;
    esac
}

interband_validate_envelope_file() {
    local source_path="${1:-}"
    [[ -n "$source_path" && -f "$source_path" ]] || return 1
    command -v jq >/dev/null 2>&1 || return 1

    jq -e '
      (.version | type == "string" and startswith("1.")) and
      (.namespace | type == "string" and length > 0) and
      (.type | type == "string" and length > 0) and
      (.session_id | type == "string") and
      (.timestamp | type == "string" and length > 0) and
      (.payload | type == "object")
    ' "$source_path" >/dev/null 2>&1
}

interband_read_envelope() {
    local source_path="${1:-}"
    interband_validate_envelope_file "$source_path" || return 1
    jq -c '.' "$source_path" 2>/dev/null
}

interband_write() {
    local target_path="${1:-}" namespace="${2:-}" type="${3:-}" session_id="${4:-}" payload_json="${5:-}"
    [[ -n "$target_path" && -n "$namespace" && -n "$type" && -n "$payload_json" ]] || return 1
    command -v jq >/dev/null 2>&1 || return 1

    interband_validate_payload "$namespace" "$type" "$payload_json" || return 1

    local target_dir
    target_dir="$(dirname "$target_path")"
    mkdir -p "$target_dir" 2>/dev/null || return 1

    local tmp_file
    tmp_file="$(mktemp "${target_dir}/.interband-tmp.XXXXXX")" || return 1

    jq -n -c \
        --arg version "$(interband_protocol_version)" \
        --arg namespace "$namespace" \
        --arg type "$type" \
        --arg session_id "$session_id" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --argjson payload "$payload_json" \
        '{version:$version,namespace:$namespace,type:$type,session_id:$session_id,timestamp:$timestamp,payload:$payload}' \
        > "$tmp_file" 2>/dev/null || {
        rm -f "$tmp_file" 2>/dev/null || true
        return 1
    }

    mv -f "$tmp_file" "$target_path" 2>/dev/null || {
        rm -f "$tmp_file" 2>/dev/null || true
        return 1
    }
}

interband_read_payload() {
    local source_path="${1:-}"
    [[ -n "$source_path" && -f "$source_path" ]] || return 1
    command -v jq >/dev/null 2>&1 || return 1

    interband_validate_envelope_file "$source_path" || return 1

    local namespace type payload_json
    namespace=$(jq -r '.namespace // ""' "$source_path" 2>/dev/null) || namespace=""
    type=$(jq -r '.type // ""' "$source_path" 2>/dev/null) || type=""
    payload_json=$(jq -c '.payload // empty' "$source_path" 2>/dev/null) || payload_json=""
    [[ -n "$payload_json" ]] || return 1

    interband_validate_payload "$namespace" "$type" "$payload_json" || return 1
    echo "$payload_json"
}
