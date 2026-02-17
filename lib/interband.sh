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

interband_channel_dir() {
    local namespace="${1:-}" channel="${2:-}"
    [[ -n "$namespace" && -n "$channel" ]] || return 1
    printf '%s/%s/%s\n' "$(interband_root)" "$namespace" "$channel"
}

interband_default_retention_secs() {
    local namespace="${1:-}" channel="${2:-}"
    case "${namespace}:${channel}" in
        clavain:dispatch)      echo "21600" ;;  # 6h
        interlock:coordination) echo "43200" ;; # 12h
        interphase:bead)       echo "86400" ;;  # 24h
        *)                     echo "86400" ;;
    esac
}

interband_default_max_files() {
    local namespace="${1:-}" channel="${2:-}"
    case "${namespace}:${channel}" in
        clavain:dispatch)      echo "128" ;;
        interlock:coordination) echo "256" ;;
        interphase:bead)       echo "256" ;;
        *)                     echo "256" ;;
    esac
}

_interband_to_env_key() {
    local raw="${1:-}"
    echo "$raw" | tr '[:lower:]-' '[:upper:]_' | sed -e 's/[^A-Z0-9_]/_/g'
}

interband_retention_secs() {
    local namespace="${1:-}" channel="${2:-}"
    local ns_key ch_key var_name
    ns_key=$(_interband_to_env_key "$namespace")
    ch_key=$(_interband_to_env_key "$channel")
    var_name="INTERBAND_RETENTION_${ns_key}_${ch_key}_SECS"

    if [[ -n "${!var_name:-}" ]]; then
        echo "${!var_name}"
    elif [[ -n "${INTERBAND_RETENTION_SECS:-}" ]]; then
        echo "${INTERBAND_RETENTION_SECS}"
    else
        interband_default_retention_secs "$namespace" "$channel"
    fi
}

interband_max_files() {
    local namespace="${1:-}" channel="${2:-}"
    local ns_key ch_key var_name
    ns_key=$(_interband_to_env_key "$namespace")
    ch_key=$(_interband_to_env_key "$channel")
    var_name="INTERBAND_MAX_FILES_${ns_key}_${ch_key}"

    if [[ -n "${!var_name:-}" ]]; then
        echo "${!var_name}"
    elif [[ -n "${INTERBAND_MAX_FILES:-}" ]]; then
        echo "${INTERBAND_MAX_FILES}"
    else
        interband_default_max_files "$namespace" "$channel"
    fi
}

interband_prune_channel() {
    local namespace="${1:-}" channel="${2:-}"
    [[ -n "$namespace" && -n "$channel" ]] || return 0

    local dir
    dir=$(interband_channel_dir "$namespace" "$channel" 2>/dev/null) || return 0
    [[ -d "$dir" ]] || return 0

    local retention_secs max_files prune_interval
    retention_secs=$(interband_retention_secs "$namespace" "$channel" 2>/dev/null || echo "")
    max_files=$(interband_max_files "$namespace" "$channel" 2>/dev/null || echo "")
    prune_interval="${INTERBAND_PRUNE_INTERVAL_SECS:-300}"

    [[ "$retention_secs" =~ ^[0-9]+$ ]] || retention_secs=$(interband_default_retention_secs "$namespace" "$channel")
    [[ "$max_files" =~ ^[0-9]+$ ]] || max_files=$(interband_default_max_files "$namespace" "$channel")
    [[ "$prune_interval" =~ ^[0-9]+$ ]] || prune_interval="300"

    local now stamp_file last_prune
    now=$(date +%s)
    stamp_file="${dir}/.interband-prune.stamp"

    if [[ -f "$stamp_file" ]]; then
        last_prune=$(stat -c %Y "$stamp_file" 2>/dev/null || echo "0")
        if [[ "$last_prune" =~ ^[0-9]+$ ]] && (( now - last_prune < prune_interval )); then
            return 0
        fi
    fi
    touch "$stamp_file" 2>/dev/null || true

    # Remove files older than retention window.
    local file mtime
    while IFS= read -r -d '' file; do
        mtime=$(stat -c %Y "$file" 2>/dev/null || echo "")
        [[ "$mtime" =~ ^[0-9]+$ ]] || continue
        if (( now - mtime > retention_secs )); then
            rm -f "$file" 2>/dev/null || true
        fi
    done < <(find "$dir" -maxdepth 1 -type f -name '*.json' -print0 2>/dev/null)

    # Enforce file count cap (keep newest files).
    if (( max_files > 0 )); then
        local idx=0
        while IFS= read -r file; do
            idx=$((idx + 1))
            if (( idx > max_files )); then
                rm -f "$file" 2>/dev/null || true
            fi
        done < <(
            find "$dir" -maxdepth 1 -type f -name '*.json' -printf '%T@ %p\n' 2>/dev/null \
                | sort -rn \
                | awk '{$1=""; sub(/^ /,""); print}'
        )
    fi
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
