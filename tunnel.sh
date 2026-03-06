#!/usr/bin/env bash
# tunnel - a friendly wrapper around devtunnel
# Install: add this one line to your ~/.bashrc:
#   source ~/.local/bin/tunnel.sh

# ─── Guard: only define once per shell session ───────────────────────────────
[[ -n "${_TUNNEL_LOADED:-}" ]] && return 0
_TUNNEL_LOADED=1

# ─── Config (prefixed to avoid polluting shell namespace) ────────────────────
_TUNNEL_LOG_DIR="${HOME}/.devtunnel/logs"
_TUNNEL_AUTH_FILE="${HOME}/.devtunnel/.auth_ok"
_TUNNEL_DEFAULT_EXPIRY="2d"

# ─── Colors ──────────────────────────────────────────────────────────────────
_T_RED='\033[0;31m'
_T_GREEN='\033[0;32m'
_T_YELLOW='\033[1;33m'
_T_CYAN='\033[0;36m'
_T_BLUE='\033[0;34m'
_T_MAGENTA='\033[0;35m'
_T_BOLD='\033[1m'
_T_DIM='\033[2m'
_T_RST='\033[0m'

# ─── Internal helpers ─────────────────────────────────────────────────────────
_t_banner()  {
  echo -e "${_T_CYAN}${_T_BOLD}"
  echo "  ╔══════════════════════════════════╗"
  echo "  ║        🚇  devtunnel             ║"
  echo "  ╚══════════════════════════════════╝"
  echo -e "${_T_RST}"
}
_t_info()    { echo -e "${_T_CYAN}  ℹ  $*${_T_RST}"; }
_t_ok()      { echo -e "${_T_GREEN}  ✔  $*${_T_RST}"; }
_t_warn()    { echo -e "${_T_YELLOW}  ⚠  $*${_T_RST}"; }
_t_err()     { echo -e "${_T_RED}  ✖  $*${_T_RST}" >&2; }
_t_section() { echo -e "\n${_T_BOLD}${_T_BLUE}── $* ──${_T_RST}"; }

_t_require() {
  if ! command -v devtunnel &>/dev/null; then
    _t_err "devtunnel not found in PATH."
    echo -e "  Install: ${_T_DIM}https://learn.microsoft.com/en-us/azure/developer/dev-tunnels/get-started${_T_RST}"
    return 1
  fi
}

_t_logfile() {
  echo "${_TUNNEL_LOG_DIR}/tunnel-${1:-host}-$(date +%Y%m%d-%H%M%S).log"
}

# ─── Auth ─────────────────────────────────────────────────────────────────────
_t_ensure_login() {
  if [[ -f "${_TUNNEL_AUTH_FILE}" ]]; then
    local age
    if [[ "$(uname)" == "Darwin" ]]; then
      age=$(( $(date +%s) - $(stat -f %m "${_TUNNEL_AUTH_FILE}") ))
    else
      age=$(( $(date +%s) - $(stat -c %Y "${_TUNNEL_AUTH_FILE}") ))
    fi
    # 6 days = 518400 seconds
    if (( age < 518400 )); then
      return 0
    fi
  fi

  _t_section "GitHub Login"
  _t_info "Token expired or missing — logging in via GitHub device code..."
  echo ""
  mkdir -p "$(dirname "${_TUNNEL_AUTH_FILE}")"

  if devtunnel user login -g -d; then
    touch "${_TUNNEL_AUTH_FILE}"
    _t_ok "Login successful — cached for 6 days."
  else
    _t_err "Login failed. Run: tunnel login"
    return 1
  fi
}

# ─── Commands ────────────────────────────────────────────────────────────────

_tunnel_login() {
  rm -f "${_TUNNEL_AUTH_FILE}"
  _t_ensure_login
}

_tunnel_logout() {
  devtunnel user logout
  rm -f "${_TUNNEL_AUTH_FILE}"
  _t_ok "Logged out and cache cleared."
}

_tunnel_whoami() { devtunnel user show; }

_tunnel_host() {
  local port="${1:-}"
  if [[ -z "${port}" ]]; then
    _t_err "Usage: tunnel <port> [expiry]"
    echo -e "  Example: ${_T_DIM}tunnel 8080${_T_RST}"
    echo -e "  Example: ${_T_DIM}tunnel 3000 7d${_T_RST}"
    return 1
  fi

  local expiry="${2:-${_TUNNEL_DEFAULT_EXPIRY}}"
  local logfile
  logfile="$(_t_logfile "${port}")"
  mkdir -p "${_TUNNEL_LOG_DIR}"
  _t_ensure_login || return 1

  _t_banner
  _t_section "Starting Tunnel"
  echo -e "  ${_T_BOLD}Port     :${_T_RST} ${_T_MAGENTA}${port}${_T_RST}"
  echo -e "  ${_T_BOLD}Expiry   :${_T_RST} ${_T_YELLOW}${expiry}${_T_RST}"
  echo -e "  ${_T_BOLD}Log      :${_T_RST} ${_T_DIM}${logfile}${_T_RST}"
  echo ""

  devtunnel host -p "${port}" --expiration "${expiry}" 2>&1 | tee "${logfile}" | \
  while IFS= read -r line; do
    if echo "${line}" | grep -qE 'https://[a-z0-9-]+\.devtunnels\.ms'; then
      local url
      url=$(echo "${line}" | grep -oE 'https://[a-zA-Z0-9./_-]+\.devtunnels\.ms[^ ]*')
      echo -e "${_T_GREEN}${_T_BOLD}  🔗  ${url}${_T_RST}"
      echo -e "  ${_T_DIM}${line}${_T_RST}"
    elif echo "${line}" | grep -qiE 'connect|ready|hosting|port'; then
      echo -e "${_T_CYAN}  ▶  ${line}${_T_RST}"
    elif echo "${line}" | grep -qiE 'error|fail|denied'; then
      echo -e "${_T_RED}  ✖  ${line}${_T_RST}"
    elif echo "${line}" | grep -qi 'warn'; then
      echo -e "${_T_YELLOW}  ⚠  ${line}${_T_RST}"
    else
      echo -e "  ${_T_DIM}${line}${_T_RST}"
    fi
  done

  echo ""
  _t_info "Session ended. Log: ${logfile}"
}

_tunnel_logs() {
  _t_section "Recent Logs (${_TUNNEL_LOG_DIR})"
  ls -lt "${_TUNNEL_LOG_DIR}"/*.log 2>/dev/null | head -10 \
    || _t_warn "No logs found in ${_TUNNEL_LOG_DIR}"
}

_tunnel_tail() {
  local latest
  latest=$(ls -t "${_TUNNEL_LOG_DIR}"/*.log 2>/dev/null | head -1)
  if [[ -z "${latest}" ]]; then
    _t_warn "No logs found."
    return 1
  fi
  _t_info "Tailing: ${latest}"
  tail -f "${latest}"
}

_tunnel_help() {
  _t_banner
  echo -e "${_T_BOLD}Usage:${_T_RST}"
  echo -e "  ${_T_GREEN}tunnel <port> [expiry]${_T_RST}       Host a tunnel  (default expiry: ${_TUNNEL_DEFAULT_EXPIRY})"
  echo -e "  ${_T_GREEN}tunnel connect <id>${_T_RST}          Connect to a tunnel by ID"
  echo ""
  echo -e "${_T_BOLD}Auth:${_T_RST}"
  echo -e "  ${_T_CYAN}tunnel login${_T_RST}                 Re-auth with GitHub (device code)"
  echo -e "  ${_T_CYAN}tunnel logout${_T_RST}                Log out and clear cached token"
  echo -e "  ${_T_CYAN}tunnel whoami${_T_RST}                Show current login info"
  echo ""
  echo -e "${_T_BOLD}Manage:${_T_RST}"
  echo -e "  ${_T_YELLOW}tunnel list${_T_RST}                  List all dev tunnels"
  echo -e "  ${_T_YELLOW}tunnel show [id]${_T_RST}             Show tunnel details"
  echo -e "  ${_T_YELLOW}tunnel delete <id>${_T_RST}           Delete a tunnel"
  echo -e "  ${_T_YELLOW}tunnel delete-all${_T_RST}            Delete all tunnels"
  echo -e "  ${_T_YELLOW}tunnel token <id>${_T_RST}            Issue an access token"
  echo -e "  ${_T_YELLOW}tunnel port <cmd>${_T_RST}            Manage tunnel ports"
  echo ""
  echo -e "${_T_BOLD}Logs:${_T_RST}"
  echo -e "  ${_T_MAGENTA}tunnel logs${_T_RST}                  List recent log files"
  echo -e "  ${_T_MAGENTA}tunnel tail${_T_RST}                  Tail the most recent log"
  echo ""
  echo -e "${_T_DIM}Logs : ${_TUNNEL_LOG_DIR}${_T_RST}"
  echo -e "${_T_DIM}Auth : ${_TUNNEL_AUTH_FILE}${_T_RST}"
}

# ─── Main ─────────────────────────────────────────────────────────────────────
tunnel() {
  _t_require || return 1

  local cmd="${1:-}"
  case "${cmd}" in
    ""|-h|--help|help) _tunnel_help ;;
    login)      shift; _tunnel_login "$@" ;;
    logout)     shift; _tunnel_logout "$@" ;;
    whoami)     shift; _tunnel_whoami "$@" ;;
    connect)    shift; _t_ensure_login && devtunnel connect "$@" ;;
    list)       shift; devtunnel list "$@" ;;
    show)       shift; devtunnel show "$@" ;;
    delete-all) shift; devtunnel delete-all "$@" ;;
    delete)     shift; devtunnel delete "$@" ;;
    token)      shift; devtunnel token "$@" ;;
    port)       shift; devtunnel port "$@" ;;
    logs)       shift; _tunnel_logs "$@" ;;
    tail)       shift; _tunnel_tail "$@" ;;
    [0-9]*)     _tunnel_host "$@" ;;
    *)
      _t_err "Unknown command: ${cmd}"
      _tunnel_help
      return 1
      ;;
  esac
}
