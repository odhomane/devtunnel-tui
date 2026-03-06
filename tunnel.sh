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
    _t_err "Usage: tunnel <port> [expiry] [--allow-anonymous] [--fg] [--flags...]"
    echo -e "  Example: ${_T_DIM}tunnel 8080${_T_RST}"
    echo -e "  Example: ${_T_DIM}tunnel 3000 7d${_T_RST}"
    echo -e "  Example: ${_T_DIM}tunnel 8080 --fg${_T_RST}        (foreground, follow logs)"
    return 1
  fi
  shift  # consume port

  # Second arg: expiry if it looks like a duration (e.g. 2d, 12h, 30d)
  local expiry="${_TUNNEL_DEFAULT_EXPIRY}"
  if [[ "${1:-}" =~ ^[0-9]+[hdm]$ ]]; then
    expiry="${1}"
    shift
  fi

  # Check for --fg flag (foreground mode)
  local foreground=0
  local extra_flags=()
  for arg in "$@"; do
    if [[ "${arg}" == "--fg" ]]; then
      foreground=1
    else
      extra_flags+=("${arg}")
    fi
  done

  local anon_label=""
  [[ " ${extra_flags[*]} " == *"--allow-anonymous"* ]] && anon_label=" ${_T_YELLOW}(anonymous)${_T_RST}"

  local logfile
  logfile="$(_t_logfile "${port}")"
  local pidfile="${_TUNNEL_LOG_DIR}/tunnel-${port}.pid"
  mkdir -p "${_TUNNEL_LOG_DIR}"
  _t_ensure_login || return 1

  # Kill any existing tunnel on this port
  if [[ -f "${pidfile}" ]]; then
    local oldpid
    oldpid=$(cat "${pidfile}")
    if kill -0 "${oldpid}" 2>/dev/null; then
      _t_warn "Killing existing tunnel on port ${port} (PID ${oldpid})"
      kill "${oldpid}" 2>/dev/null || true
      sleep 1
    fi
    rm -f "${pidfile}"
  fi

  _t_banner
  _t_section "Starting Tunnel"
  echo -e "  ${_T_BOLD}Port     :${_T_RST} ${_T_MAGENTA}${port}${_T_RST}${anon_label}"
  echo -e "  ${_T_BOLD}Expiry   :${_T_RST} ${_T_YELLOW}${expiry}${_T_RST}"
  [[ ${#extra_flags[@]} -gt 0 ]] && echo -e "  ${_T_BOLD}Flags    :${_T_RST} ${_T_DIM}${extra_flags[*]}${_T_RST}"
  echo -e "  ${_T_BOLD}Log      :${_T_RST} ${_T_DIM}${logfile}${_T_RST}"
  echo ""

  if [[ "${foreground}" -eq 1 ]]; then
    # Foreground: stream output live
    _t_info "Running in foreground (Ctrl+C to stop)..."
    devtunnel host -p "${port}" --expiration "${expiry}" "${extra_flags[@]}" 2>&1 | tee "${logfile}" |     _t_render_tunnel_output
    echo ""
    _t_info "Session ended. Log: ${logfile}"
  else
    # Background: launch detached, tail until URL appears then return shell
    _t_info "Launching in background..."
    devtunnel host -p "${port}" --expiration "${expiry}" "${extra_flags[@]}" > "${logfile}" 2>&1 &
    local bgpid=$!
    echo "${bgpid}" > "${pidfile}"

    # Wait up to 15s for the tunnel URL to appear in the log
    local waited=0
    local url=""
    while (( waited < 15 )); do
      sleep 1
      (( waited++ ))
      if ! kill -0 "${bgpid}" 2>/dev/null; then
        _t_err "Tunnel process exited early. Check log: ${logfile}"
        tail -5 "${logfile}" | while IFS= read -r line; do echo -e "  ${_T_DIM}${line}${_T_RST}"; done
        rm -f "${pidfile}"
        return 1
      fi
      url=$(grep -oE 'https://[a-zA-Z0-9._-]+\.devtunnels\.ms[^ ]*' "${logfile}" 2>/dev/null | grep -v inspect | head -1 || true)
      [[ -n "${url}" ]] && break
    done

    if [[ -n "${url}" ]]; then
      local inspect_url
      inspect_url=$(grep -oE 'https://[a-zA-Z0-9._-]+-inspect\.[^ ]*' "${logfile}" 2>/dev/null | head -1 || true)
      echo -e "${_T_GREEN}${_T_BOLD}  🔗  Browser  → ${url}${_T_RST}"
      [[ -n "${inspect_url}" ]] && echo -e "${_T_CYAN}  🔍 Inspect  → ${inspect_url}${_T_RST}"
      echo ""
      _t_ok "Tunnel running in background  PID=${bgpid}  expires in ${expiry}"
      _t_info "Logs : tunnel tail"
      _t_info "Stop : tunnel stop ${port}"
    else
      _t_warn "Tunnel started (PID=${bgpid}) but URL not yet visible — check: tunnel tail"
    fi
  fi
}

_t_render_tunnel_output() {
  while IFS= read -r line; do
    local url
    url=$(echo "${line}" | grep -oE 'https://[a-zA-Z0-9._-]+\.devtunnels\.ms[^ ]*' || true)
    if [[ "${line}" == *"Connect via browser"* ]] && [[ -n "${url}" ]]; then
      echo -e "${_T_GREEN}${_T_BOLD}  🔗  Browser  → ${url}${_T_RST}"
    elif [[ "${line}" == *"inspect"* ]] && [[ -n "${url}" ]]; then
      echo -e "${_T_CYAN}  🔍 Inspect  → ${url}${_T_RST}"
    elif [[ -n "${url}" ]]; then
      echo -e "${_T_GREEN}${_T_BOLD}  🔗  ${url}${_T_RST}"
    elif echo "${line}" | grep -qiE 'ready|accept|tunnel:'; then
      echo -e "${_T_GREEN}  ✔  ${line}${_T_RST}"
    elif echo "${line}" | grep -qiE 'hosting port|connect'; then
      echo -e "${_T_CYAN}  ▶  ${line}${_T_RST}"
    elif echo "${line}" | grep -qiE 'error|fail|denied'; then
      echo -e "${_T_RED}  ✖  ${line}${_T_RST}"
    elif echo "${line}" | grep -qi 'warn'; then
      echo -e "${_T_YELLOW}  ⚠  ${line}${_T_RST}"
    else
      echo -e "  ${_T_DIM}${line}${_T_RST}"
    fi
  done
}

_tunnel_stop() {
  local port="${1:-}"
  local pidfile="${_TUNNEL_LOG_DIR}/tunnel-${port}.pid"

  if [[ -z "${port}" ]]; then
    # Stop all
    local stopped=0
    for pf in "${_TUNNEL_LOG_DIR}"/*.pid; do
      [[ -f "${pf}" ]] || continue
      local pid
      pid=$(cat "${pf}")
      local p
      p=$(basename "${pf}" .pid | sed 's/tunnel-//')
      if kill "${pid}" 2>/dev/null; then
        _t_ok "Stopped tunnel on port ${p} (PID ${pid})"
        (( stopped++ ))
      fi
      rm -f "${pf}"
    done
    [[ "${stopped}" -eq 0 ]] && _t_warn "No running tunnels found."
    return 0
  fi

  if [[ ! -f "${pidfile}" ]]; then
    _t_warn "No PID file for port ${port}. Try: tunnel ps"
    return 1
  fi
  local pid
  pid=$(cat "${pidfile}")
  if kill "${pid}" 2>/dev/null; then
    _t_ok "Stopped tunnel on port ${port} (PID ${pid})"
    rm -f "${pidfile}"
  else
    _t_warn "Process ${pid} not found (already stopped?)"
    rm -f "${pidfile}"
  fi
}

_tunnel_ps() {
  local found=0
  for pf in "${_TUNNEL_LOG_DIR}"/*.pid; do
    [[ -f "${pf}" ]] || continue
    local pid
    pid=$(cat "${pf}")
    local port
    port=$(basename "${pf}" .pid | sed 's/tunnel-//')
    if kill -0 "${pid}" 2>/dev/null; then
      local url
      url=$(grep -oE 'https://[a-zA-Z0-9._-]+\.devtunnels\.ms[^ ]*'         "${_TUNNEL_LOG_DIR}/$(ls -t "${_TUNNEL_LOG_DIR}" | grep "tunnel-${port}-" | head -1)"         2>/dev/null | grep -v inspect | head -1 || true)
      echo -e "  ${_T_GREEN}${_T_BOLD}●${_T_RST}  port ${_T_MAGENTA}${port}${_T_RST}  PID=${_T_DIM}${pid}${_T_RST}  ${_T_GREEN}${url}${_T_RST}"
      (( found++ ))
    else
      _t_warn "Stale PID file for port ${port} (PID ${pid} gone) — cleaning up"
      rm -f "${pf}"
    fi
  done
  [[ "${found}" -eq 0 ]] && _t_warn "No running tunnels."
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
  echo -e "  ${_T_GREEN}tunnel <port> [expiry] [flags]${_T_RST}  Host tunnel in background (default expiry: ${_TUNNEL_DEFAULT_EXPIRY})"
  echo -e "  ${_T_DIM}         --fg                    Run in foreground (stream logs)${_T_RST}"
  echo -e "  ${_T_DIM}         --allow-anonymous       Public access (no login required)${_T_RST}"
  echo -e "  ${_T_DIM}         --protocol https        Set port protocol${_T_RST}"
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
  echo -e "${_T_BOLD}Background:${_T_RST}"
  echo -e "  ${_T_MAGENTA}tunnel ps${_T_RST}                    Show running tunnels"
  echo -e "  ${_T_MAGENTA}tunnel stop [port]${_T_RST}           Stop tunnel (omit port = stop all)"
  echo -e "  ${_T_MAGENTA}tunnel logs${_T_RST}                  List recent log files"
  echo -e "  ${_T_MAGENTA}tunnel tail${_T_RST}                  Tail the most recent log"
  echo ""
  echo -e "${_T_DIM}Logs : ${_TUNNEL_LOG_DIR}${_T_RST}"
  echo -e "${_T_DIM}Auth : ${_TUNNEL_AUTH_FILE}${_T_RST}"
}

# ─── Main ─────────────────────────────────────────────────────────────────────
tunnel() {
  local cmd="${1:-}"
  case "${cmd}" in
    ""|-h|--help|help) _tunnel_help; return 0 ;;
  esac
  _t_require || return 1
  case "${cmd}" in
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
    stop)       shift; _tunnel_stop "$@" ;;
    ps)         shift; _tunnel_ps "$@" ;;
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
