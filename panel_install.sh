#!/usr/bin/env bash
set -Eeuo pipefail

REPOSITORY="kukumi1/flux-panel-plus"
BRANCH="${FLUX_BRANCH:-main}"
INSTALL_DIR="${FLUX_INSTALL_DIR:-/opt/flux-panel-plus}"
BACKUP_DIR="${FLUX_BACKUP_DIR:-${INSTALL_DIR}/backups}"
RELEASE_BASE="https://github.com/${REPOSITORY}/releases/latest/download"
RAW_BASE="https://raw.githubusercontent.com/${REPOSITORY}/${BRANCH}"
COMPOSE_FILE="${INSTALL_DIR}/docker-compose.yml"
ENV_FILE="${INSTALL_DIR}/.env"

log() {
  printf '[flux-panel-plus] %s\n' "$*"
}

warn() {
  printf '[flux-panel-plus] WARNING: %s\n' "$*" >&2
}

die() {
  printf '[flux-panel-plus] ERROR: %s\n' "$*" >&2
  exit 1
}

require_root() {
  if [[ ${EUID} -ne 0 ]]; then
    die "Please run with sudo or as root."
  fi
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

confirm() {
  local prompt="$1"
  local answer
  if [[ "${FLUX_ASSUME_YES:-0}" == "1" ]]; then
    return 0
  fi
  [[ -t 0 ]] || return 1
  read -r -p "${prompt} [y/N]: " answer
  [[ "${answer}" =~ ^[Yy]$ ]]
}

install_docker() {
  require_command curl
  local installer
  installer="$(mktemp)"
  log "Downloading the official Docker installation script..."
  if ! curl -fsSL https://get.docker.com -o "${installer}"; then
    rm -f "${installer}"
    die "Failed to download the Docker installation script."
  fi
  if ! sh "${installer}"; then
    rm -f "${installer}"
    die "Docker installation failed."
  fi
  rm -f "${installer}"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable --now docker
  fi
}

ensure_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    if [[ "${FLUX_INSTALL_DOCKER:-0}" == "1" ]] || confirm "Docker is not installed. Install it now?"; then
      install_docker
    else
      die "Docker is required. Install Docker Engine and rerun this script."
    fi
  fi
  docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is required."
}

compose() {
  (cd "${INSTALL_DIR}" && docker compose "$@")
}

sha256_check() {
  local file="$1"
  local asset_name="$2"
  local checksums expected actual
  checksums="$(mktemp)"
  if ! curl -fsSL "${RELEASE_BASE}/SHA256SUMS" -o "${checksums}"; then
    rm -f "${checksums}"
    return 0
  fi
  expected="$(awk -v name="${asset_name}" '$2 == name {print $1; exit}' "${checksums}")"
  rm -f "${checksums}"
  [[ -n "${expected}" ]] || return 0
  actual="$(sha256sum "${file}" | awk '{print $1}')"
  [[ "${actual}" == "${expected}" ]] || die "Checksum verification failed for ${asset_name}."
}

download_asset() {
  local asset_name="$1"
  local destination="$2"
  local raw_name="${3:-$1}"
  local temporary
  temporary="$(mktemp)"

  if curl -fsSL "${RELEASE_BASE}/${asset_name}" -o "${temporary}"; then
    sha256_check "${temporary}" "${asset_name}"
    log "Downloaded ${asset_name} from the latest release."
  else
    warn "Release asset ${asset_name} is unavailable; falling back to ${BRANCH}."
    curl -fsSL "${RAW_BASE}/${raw_name}" -o "${temporary}"
  fi

  install -m 0644 "${temporary}" "${destination}"
  rm -f "${temporary}"
}

random_value() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
  else
    tr -dc 'A-Za-z0-9' </dev/urandom | head -c 48
  fi
}

prompt_port() {
  local port="${PANEL_PORT:-6366}"
  if [[ -t 0 && -z "${PANEL_PORT:-}" ]]; then
    read -r -p "Panel port [6366]: " port
    port="${port:-6366}"
  fi
  [[ "${port}" =~ ^[0-9]+$ ]] || die "Panel port must be numeric."
  ((port >= 1 && port <= 65535)) || die "Panel port must be between 1 and 65535."
  printf '%s' "${port}"
}

create_env() {
  if [[ -f "${ENV_FILE}" ]]; then
    log "Keeping existing ${ENV_FILE}."
    return
  fi

  local panel_port
  panel_port="$(prompt_port)"
  umask 077
  cat >"${ENV_FILE}" <<EOF
COMPOSE_PROJECT_NAME=flux-panel-plus
FLUX_VERSION=latest
DB_NAME=flux_db
DB_USER=flux_user
DB_PASSWORD=$(random_value)
JWT_SECRET=$(random_value)
PANEL_LISTEN=0.0.0.0
PANEL_PORT=${panel_port}
ENABLE_IPV6=false
ALLOWED_ORIGINS=
TZ=Asia/Shanghai
EOF
  chmod 0600 "${ENV_FILE}"
  log "Created ${ENV_FILE} with random credentials."
}

env_value() {
  local key="$1"
  awk -F= -v key="${key}" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "${ENV_FILE}"
}

wait_for_backend() {
  local timeout="${1:-180}"
  local status=""
  local elapsed=0
  while ((elapsed < timeout)); do
    status="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' go-backend 2>/dev/null || true)"
    if [[ "${status}" == "healthy" ]]; then
      log "Backend health check passed."
      return 0
    fi
    if [[ "${status}" == "unhealthy" || "${status}" == "exited" ]]; then
      warn "Backend status: ${status}"
      return 1
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  warn "Backend did not become healthy within ${timeout} seconds."
  return 1
}

download_deployment_files() {
  mkdir -p "${INSTALL_DIR}"
  download_asset docker-compose.yml "${COMPOSE_FILE}"
  download_asset env.example "${INSTALL_DIR}/.env.example" .env.example
}

show_access_info() {
  local port
  port="$(env_value PANEL_PORT)"
  log "Panel URL: http://SERVER_IP:${port:-6366}"
  log "Default administrator username: admin_user"
  log "Read the generated administrator password with:"
  printf '  docker logs go-backend 2>&1 | grep -E "password|密码"\n'
}

install_panel() {
  require_root
  require_command curl
  require_command sha256sum
  ensure_docker
  download_deployment_files
  create_env

  log "Pulling Flux Panel Plus images..."
  compose pull
  log "Starting services..."
  compose up -d --remove-orphans
  if ! wait_for_backend 180; then
    compose ps
    compose logs --tail=120 backend
    die "Installation failed because the backend is not healthy."
  fi
  show_access_info
}

backup_panel() {
  require_root
  ensure_docker
  [[ -f "${ENV_FILE}" ]] || die "Missing ${ENV_FILE}."
  [[ -f "${COMPOSE_FILE}" ]] || die "Missing ${COMPOSE_FILE}."

  local timestamp work archive db_name db_user db_password
  timestamp="$(date +%Y%m%d_%H%M%S)"
  mkdir -p "${BACKUP_DIR}"
  work="${BACKUP_DIR}/flux-panel-plus-${timestamp}"
  archive="${work}.tar.gz"
  mkdir -p "${work}"
  cp "${ENV_FILE}" "${COMPOSE_FILE}" "${work}/"

  db_name="$(env_value DB_NAME)"
  db_user="$(env_value DB_USER)"
  db_password="$(env_value DB_PASSWORD)"
  if docker inspect flux-mysql >/dev/null 2>&1; then
    log "Exporting MySQL database..."
    docker exec -e MYSQL_PWD="${db_password}" flux-mysql \
      mysqldump -u"${db_user}" --single-transaction --routines --triggers "${db_name}" \
      >"${work}/database.sql"
  else
    warn "MySQL container not found; configuration files will still be backed up."
  fi

  tar -C "${BACKUP_DIR}" -czf "${archive}" "$(basename "${work}")"
  chmod 0600 "${archive}"
  rm -rf -- "${work}"
  log "Backup created: ${archive}"
}

update_panel() {
  require_root
  require_command curl
  require_command sha256sum
  ensure_docker
  [[ -f "${ENV_FILE}" ]] || die "Missing ${ENV_FILE}; run install first."

  backup_panel
  cp "${COMPOSE_FILE}" "${COMPOSE_FILE}.pre-update"
  download_deployment_files

  log "Pulling updated images..."
  compose pull
  compose up -d --remove-orphans
  if wait_for_backend 180; then
    rm -f "${COMPOSE_FILE}.pre-update"
    log "Update completed."
    return
  fi

  warn "Update failed; restoring the previous Compose file."
  mv -f "${COMPOSE_FILE}.pre-update" "${COMPOSE_FILE}"
  compose up -d --remove-orphans
  wait_for_backend 180 || true
  die "Update failed and the previous Compose file was restored."
}

show_status() {
  require_root
  ensure_docker
  [[ -f "${COMPOSE_FILE}" ]] || die "Missing ${COMPOSE_FILE}."
  compose ps
}

show_logs() {
  require_root
  ensure_docker
  [[ -f "${COMPOSE_FILE}" ]] || die "Missing ${COMPOSE_FILE}."
  compose logs --tail=200 -f
}

uninstall_panel() {
  require_root
  ensure_docker
  [[ -f "${COMPOSE_FILE}" ]] || die "Missing ${COMPOSE_FILE}."

  local purge="${1:-}"
  if [[ "${purge}" == "--purge" ]]; then
    confirm "Permanently remove containers and database volumes?" || die "Uninstall cancelled."
    compose down --volumes --remove-orphans
    rm -f "${COMPOSE_FILE}" "${ENV_FILE}" "${INSTALL_DIR}/.env.example"
    log "Containers, volumes, and deployment configuration were removed. Backups were kept."
    return
  fi

  confirm "Stop and remove Flux Panel Plus containers? Database volumes will be kept." || die "Uninstall cancelled."
  compose down --remove-orphans
  log "Containers were removed. Data volumes and files in ${INSTALL_DIR} were kept."
}

show_menu() {
  cat <<'EOF'

Flux Panel Plus
1) Install
2) Update
3) Backup
4) Status
5) Logs
6) Uninstall (keep data)
7) Purge (delete data volumes)
0) Exit
EOF
}

interactive_menu() {
  local choice
  while true; do
    show_menu
    read -r -p "Select an action: " choice
    case "${choice}" in
      1) install_panel; return ;;
      2) update_panel; return ;;
      3) backup_panel; return ;;
      4) show_status; return ;;
      5) show_logs; return ;;
      6) uninstall_panel; return ;;
      7) uninstall_panel --purge; return ;;
      0) return ;;
      *) warn "Invalid selection." ;;
    esac
  done
}

usage() {
  cat <<EOF
Usage: $0 <command>

Commands:
  install             Install or start Flux Panel Plus
  update              Back up and update to the latest release
  backup              Back up MySQL and deployment configuration
  status              Show container status
  logs                Follow service logs
  uninstall           Remove containers but keep data volumes
  uninstall --purge   Remove containers and data volumes
  menu                Open the interactive menu

Environment overrides:
  FLUX_INSTALL_DIR=/opt/flux-panel-plus
  FLUX_BACKUP_DIR=/opt/flux-panel-plus/backups
  PANEL_PORT=6366
  FLUX_INSTALL_DOCKER=1
  FLUX_ASSUME_YES=1
EOF
}

main() {
  local action="${1:-menu}"
  case "${action}" in
    install) install_panel ;;
    update) update_panel ;;
    backup) backup_panel ;;
    status) show_status ;;
    logs) show_logs ;;
    uninstall) uninstall_panel "${2:-}" ;;
    menu) interactive_menu ;;
    -h|--help|help) usage ;;
    *) usage; exit 2 ;;
  esac
}

main "$@"
