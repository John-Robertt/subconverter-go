#!/usr/bin/env bash
set -euo pipefail

# 生成“真实订阅”产物：
# - 使用本地 SS.txt（仓库根目录，已在 .gitignore 中忽略）
# - 使用 docs/materials/profile_rules_ini.yaml（模板/规则集为远程 URL）
# - 输出到 out/real/
#
# 依赖：
# - python3（启动静态文件服务器）
# - go（启动转换服务）
# - curl（拉取结果）

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

DOCS_PORT="${DOCS_PORT:-8000}"
SUB_PORT="${SUB_PORT:-8001}"
API_PORT="${API_PORT:-25500}"

DOCS_BASE="http://127.0.0.1:${DOCS_PORT}"
SUB_BASE="http://127.0.0.1:${SUB_PORT}"
API_BASE="http://127.0.0.1:${API_PORT}"

SUB_URL="${SUB_BASE}/SS.txt"
PROFILE_URL="${DOCS_BASE}/materials/profile_rules_ini.yaml"

OUT_DIR="${ROOT}/out/real"
mkdir -p "${OUT_DIR}"

die() {
  echo "error: $*" 1>&2
  exit 1
}

wait_http() {
  local url="$1"
  local name="$2"
  local i
  for i in $(seq 1 50); do
    if curl -fsS -I "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  die "${name} 未就绪：${url}"
}

cleanup() {
  # Best-effort cleanup; ignore errors.
  if [[ -n "${API_PID:-}" ]]; then kill "${API_PID}" >/dev/null 2>&1 || true; fi
  if [[ -n "${SUB_PID:-}" ]]; then kill "${SUB_PID}" >/dev/null 2>&1 || true; fi
  if [[ -n "${DOCS_PID:-}" ]]; then kill "${DOCS_PID}" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT

[[ -f "${ROOT}/SS.txt" ]] || die "缺少真实订阅文件：${ROOT}/SS.txt"
[[ -f "${ROOT}/docs/materials/profile_rules_ini.yaml" ]] || die "缺少 profile：${ROOT}/docs/materials/profile_rules_ini.yaml"

echo "[1/4] 启动静态文件服务器（docs -> ${DOCS_BASE}）"
python3 -m http.server "${DOCS_PORT}" --directory "${ROOT}/docs" >/dev/null 2>&1 &
DOCS_PID="$!"
wait_http "${DOCS_BASE}/materials/templates/clash.yaml" "docs 静态服务器"

echo "[2/4] 启动静态文件服务器（repo root -> ${SUB_BASE}，提供 SS.txt）"
python3 -m http.server "${SUB_PORT}" --directory "${ROOT}" >/dev/null 2>&1 &
SUB_PID="$!"
wait_http "${SUB_URL}" "订阅静态服务器"

echo "[3/4] 启动转换服务（${API_BASE}）"
go build -o "${ROOT}/subconverter-go" ./cmd/subconverter-go >/dev/null
"${ROOT}/subconverter-go" -listen "127.0.0.1:${API_PORT}" >/dev/null 2>&1 &
API_PID="$!"
wait_http "${API_BASE}/healthz" "转换服务"

echo "[4/4] 拉取并写入 out/real（profile=${PROFILE_URL} sub=${SUB_URL}）"

curl_to_file() {
  local outfile="$1"
  shift

  local tmp
  tmp="$(mktemp)"
  local code=""
  local attempt

  for attempt in 1 2 3 4 5; do
    # NOTE: do not use -f here; we want the JSON error body for diagnostics.
    if code="$(curl -sS -o "${tmp}" -w "%{http_code}" "$@" 2>/dev/null || true)"; then
      :
    fi
    case "${code}" in
      200)
        mv "${tmp}" "${outfile}"
        return 0
        ;;
      502|504|"")
        echo "warn: fetch failed (http=${code:-curl-error}) retry=${attempt}/5 file=$(basename "${outfile}")" 1>&2
        sleep 0.4
        ;;
      *)
        echo "error: http=${code} file=$(basename "${outfile}")" 1>&2
        head -n 80 "${tmp}" 1>&2
        rm -f "${tmp}"
        exit 1
        ;;
    esac
  done

  echo "error: retries exhausted file=$(basename "${outfile}")" 1>&2
  head -n 80 "${tmp}" 1>&2
  rm -f "${tmp}"
  exit 1
}

gen_config() {
  local target="$1"
  local outfile="$2"
  echo " - target=${target} -> $(basename "${outfile}")" 1>&2
  curl_to_file "${outfile}" --get "${API_BASE}/sub" \
    --data-urlencode "mode=config" \
    --data-urlencode "target=${target}" \
    --data-urlencode "sub=${SUB_URL}" \
    --data-urlencode "profile=${PROFILE_URL}"
}

gen_list_raw() {
  local outfile="$1"
  echo " - mode=list(raw) -> $(basename "${outfile}")" 1>&2
  curl_to_file "${outfile}" --get "${API_BASE}/sub" \
    --data-urlencode "mode=list" \
    --data-urlencode "encode=raw" \
    --data-urlencode "sub=${SUB_URL}"
}

gen_config "clash" "${OUT_DIR}/clash.yaml"
gen_config "surge" "${OUT_DIR}/surge.conf"
gen_config "quanx" "${OUT_DIR}/quanx.conf"
gen_config "shadowrocket" "${OUT_DIR}/shadowrocket.conf"
gen_list_raw "${OUT_DIR}/ss.txt"

echo "ok: ${OUT_DIR}"
