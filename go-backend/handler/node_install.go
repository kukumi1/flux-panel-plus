package handler

import (
	"flux-panel/go-backend/config"
	"flux-panel/go-backend/model"
	"flux-panel/go-backend/service"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func NodeInstallScript(c *gin.Context) {
	script := `#!/bin/bash
set -e

NODE_ID=$1
NODE_SECRET=$2
PANEL_ADDR=$3
USE_IPV6=$4

if [ -z "$NODE_ID" ] || [ -z "$NODE_SECRET" ] || [ -z "$PANEL_ADDR" ]; then
    echo "Usage: $0 <node_id> <node_secret> <panel_addr> [6]"
    exit 1
fi

CURL_FLAGS="-fsSL"
if [ "$USE_IPV6" = "6" ]; then
    CURL_FLAGS="-6fsSL"
    echo "IPv6 mode enabled"
fi

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l) ARCH="arm" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Stop existing service if running (avoid "text file busy" on overwrite)
if systemctl is-active --quiet gost-node 2>/dev/null; then
    echo "Stopping existing gost-node service..."
    systemctl stop gost-node
fi
rm -f /usr/local/bin/gost-node

# Download binary
echo "Downloading gost-node for $ARCH..."
curl $CURL_FLAGS "$PANEL_ADDR/node-install/binary/$ARCH" -o /usr/local/bin/gost-node
chmod +x /usr/local/bin/gost-node

# Create data directory
mkdir -p /etc/gost

# Install Xray from panel
if [ -x /usr/local/bin/xray ]; then
    echo "Xray already installed, skipping..."
else
    echo "Installing Xray for $ARCH..."
    curl $CURL_FLAGS "$PANEL_ADDR/node-install/xray/$ARCH" -o /usr/local/bin/xray || { echo "Warning: Xray download failed, skipping"; }
    if [ -f /usr/local/bin/xray ]; then
        chmod +x /usr/local/bin/xray
        cp /usr/local/bin/xray /etc/gost/xray
        echo "Xray installed: $(/usr/local/bin/xray version 2>/dev/null | head -1)"
    fi
fi

# Detect TLS from panel address
USE_TLS=false
ADDR_VALUE="$PANEL_ADDR"
case "$ADDR_VALUE" in
    https://*) USE_TLS=true ;;
esac
ADDR_VALUE="${ADDR_VALUE#http://}"
ADDR_VALUE="${ADDR_VALUE#https://}"
ADDR_VALUE="${ADDR_VALUE%/}"

# Generate config.json
cat > /etc/gost/config.json << EOF
{
  "addr": "$ADDR_VALUE",
  "secret": "$NODE_SECRET",
  "use_tls": $USE_TLS
}
EOF

# Ensure runtime config file exists (migrate legacy gost.json → runtime.json)
if [ -f /etc/gost/gost.json ] && [ ! -f /etc/gost/runtime.json ]; then
    mv /etc/gost/gost.json /etc/gost/runtime.json
elif [ ! -f /etc/gost/runtime.json ]; then
    echo "{}" > /etc/gost/runtime.json
fi
rm -f /etc/gost/gost.json

# Create systemd service
cat > /etc/systemd/system/gost-node.service << EOF
[Unit]
Description=GOST Node
After=network.target

[Service]
Type=simple
WorkingDirectory=/etc/gost
ExecStart=/usr/local/bin/gost-node
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable gost-node
systemctl restart gost-node

echo "GOST Node installed and started successfully!"
echo "Node ID: $NODE_ID"
echo "Config: /etc/gost/config.json"
echo "Logs: journalctl -u gost-node -f"
`
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, script)
}

var allowedArchs = map[string]bool{
	"amd64": true,
	"arm64": true,
	"arm":   true,
}

func NodeInstallBinary(c *gin.Context) {
	arch := c.Param("arch")
	if !allowedArchs[arch] {
		c.String(http.StatusBadRequest, "invalid architecture")
		return
	}

	binaryPath := filepath.Join(config.Cfg.NodeBinaryDir, fmt.Sprintf("node-%s", arch))

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "Binary not found for architecture: "+arch)
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=node-%s", arch))
	c.File(binaryPath)
}

func NodeUninstallScript(c *gin.Context) {
	script := `#!/bin/bash
set -e

echo "Stopping gost-node service..."
systemctl stop gost-node 2>/dev/null || true
systemctl disable gost-node 2>/dev/null || true
rm -f /etc/systemd/system/gost-node.service
systemctl daemon-reload

echo "Removing binaries..."
rm -f /usr/local/bin/gost-node
rm -f /usr/local/bin/xray

echo "Removing config directory..."
rm -rf /etc/gost

echo "GOST Node uninstalled successfully!"
`
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, script)
}

func NodeInstallXray(c *gin.Context) {
	arch := c.Param("arch")
	if !allowedArchs[arch] {
		c.String(http.StatusBadRequest, "invalid architecture")
		return
	}

	binaryPath := filepath.Join(config.Cfg.NodeBinaryDir, fmt.Sprintf("xray-%s", arch))

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "Xray binary not found for architecture: "+arch)
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=svc-%s", arch))
	c.File(binaryPath)
}

// ─── Camouflaged install handlers ───

// findNodeBySecret looks up a node by its secret.
func findNodeBySecret(secret string) *model.Node {
	var node model.Node
	if err := service.DB.Where("secret = ?", secret).First(&node).Error; err != nil {
		return nil
	}
	return &node
}

func scriptShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func firstHeaderValue(value string) string {
	if idx := strings.Index(value, ","); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func isLocalPanelHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}
	return strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "[::1]") || strings.HasPrefix(host, "::1")
}

func normalizeInstallPanelAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimSuffix(addr, "/")
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

func installPanelAddress(c *gin.Context) string {
	if panel := strings.TrimSpace(c.Query("panel")); panel != "" {
		return normalizeInstallPanelAddr(panel)
	}
	if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" {
		return service.GetPanelAddress(origin)
	}

	configured := service.GetPanelAddress("")
	if !isLocalPanelHost(configured) {
		return configured
	}

	host := firstHeaderValue(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	if host == "" {
		return configured
	}

	proto := strings.ToLower(firstHeaderValue(c.GetHeader("X-Forwarded-Proto")))
	if proto != "http" && proto != "https" {
		if c.Request.TLS != nil {
			proto = "https"
		} else if isLocalPanelHost(host) {
			proto = "http"
		} else {
			proto = "https"
		}
	} else if proto == "http" && !isLocalPanelHost(host) {
		proto = "https"
	}

	return proto + "://" + host
}

// CamoInstallScript generates a fully pre-configured install script with disguised paths.
// The secret in the URL path serves as authentication.
func CamoInstallScript(c *gin.Context) {
	secret := c.Param("secret")
	node := findNodeBySecret(secret)
	if node == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}

	// Ensure disguise names exist (backfill for legacy nodes)
	disguise := node.DisguiseName
	xrayDisguise := node.XrayDisguiseName
	if disguise == "" {
		disguise = "gost-node"
	}
	if xrayDisguise == "" {
		xrayDisguise = "xray"
	}

	panelAddr := installPanelAddress(c)

	// Detect TLS from panel address
	useTLS := strings.HasPrefix(panelAddr, "https://")

	// Strip scheme for config addr
	addrValue := panelAddr
	addrValue = strings.TrimPrefix(addrValue, "http://")
	addrValue = strings.TrimPrefix(addrValue, "https://")
	addrValue = strings.TrimSuffix(addrValue, "/")

	script := fmt.Sprintf(`#!/bin/bash
set -e

# ─── Camouflaged Node Install Script ───
DISGUISE="%s"
XRAY_DISGUISE="%s"
NODE_SECRET="%s"
PANEL_ADDR="%s"
USE_TLS=%t

CURL_FLAGS="-fsSL"
if [ "${1}" = "6" ]; then
    CURL_FLAGS="-6fsSL"
    echo "IPv6 mode enabled"
fi

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l) ARCH="arm" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Stop existing service if running
if systemctl is-active --quiet "$DISGUISE" 2>/dev/null; then
    echo "Stopping existing $DISGUISE service..."
    systemctl stop "$DISGUISE"
fi
# Also stop legacy gost-node service if present (migration)
if systemctl is-active --quiet gost-node 2>/dev/null; then
    echo "Stopping legacy service..."
    systemctl stop gost-node
    systemctl disable gost-node 2>/dev/null || true
    rm -f /etc/systemd/system/gost-node.service
fi
# Clean up legacy installation
rm -f /usr/local/bin/gost-node
rm -f /usr/local/bin/xray
rm -rf /etc/gost
rm -f "/usr/local/bin/$DISGUISE"

# Download binary
echo "Downloading service binary for $ARCH..."
curl $CURL_FLAGS "$PANEL_ADDR/s/$NODE_SECRET/b/$ARCH" -o "/usr/local/bin/$DISGUISE"
chmod +x "/usr/local/bin/$DISGUISE"

# Create config directory
mkdir -p "/etc/$DISGUISE"

# Install secondary binary
if [ -x "/usr/local/bin/$XRAY_DISGUISE" ]; then
    echo "Secondary binary already installed, skipping..."
else
    echo "Installing secondary binary for $ARCH..."
    curl $CURL_FLAGS "$PANEL_ADDR/s/$NODE_SECRET/x/$ARCH" -o "/usr/local/bin/$XRAY_DISGUISE" || { echo "Warning: Secondary binary download failed, skipping"; }
    if [ -f "/usr/local/bin/$XRAY_DISGUISE" ]; then
        chmod +x "/usr/local/bin/$XRAY_DISGUISE"
        cp "/usr/local/bin/$XRAY_DISGUISE" "/etc/$DISGUISE/$XRAY_DISGUISE"
        echo "Secondary binary installed"
    fi
fi

# Strip scheme for config addr
ADDR_VALUE="%s"

# Generate config.json
cat > "/etc/$DISGUISE/config.json" << EOF
{
  "addr": "$ADDR_VALUE",
  "secret": "$NODE_SECRET",
  "use_tls": $USE_TLS,
  "v_bin": "$XRAY_DISGUISE",
  "v_cfg": "service.json"
}
EOF

# Ensure runtime config file exists
if [ ! -f "/etc/$DISGUISE/runtime.json" ]; then
    echo "{}" > "/etc/$DISGUISE/runtime.json"
fi

# Create systemd service
cat > "/etc/systemd/system/$DISGUISE.service" << EOF
[Unit]
Description=$DISGUISE daemon
After=network.target

[Service]
Type=simple
WorkingDirectory=/etc/$DISGUISE
ExecStart=/usr/local/bin/$DISGUISE
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Create uninstall script
cat > "/etc/$DISGUISE/uninstall.sh" << 'UNINSTALL'
#!/bin/bash
set -e
DISGUISE="%s"
XRAY_DISGUISE="%s"
echo "Stopping service..."
systemctl stop "$DISGUISE" 2>/dev/null || true
systemctl disable "$DISGUISE" 2>/dev/null || true
rm -f "/etc/systemd/system/$DISGUISE.service"
systemctl daemon-reload
echo "Removing binaries..."
rm -f "/usr/local/bin/$DISGUISE"
rm -f "/usr/local/bin/$XRAY_DISGUISE"
echo "Removing config directory..."
rm -rf "/etc/$DISGUISE"
echo "Uninstalled successfully!"
UNINSTALL
chmod +x "/etc/$DISGUISE/uninstall.sh"

systemctl daemon-reload
systemctl enable "$DISGUISE"
systemctl restart "$DISGUISE"

echo "Service installed and started successfully!"
echo "Config: /etc/$DISGUISE/config.json"
echo "Logs: journalctl -u $DISGUISE -f"
echo "Uninstall: bash /etc/$DISGUISE/uninstall.sh"
`, disguise, xrayDisguise, node.Secret, panelAddr, useTLS, addrValue, disguise, xrayDisguise)

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, script)
}

// CamoLiteInstallScript generates a lightweight installer for small NAT nodes.
// It installs only the node agent and uses /bin/true for the optional secondary process.
func CamoLiteInstallScript(c *gin.Context) {
	secret := c.Param("secret")
	node := findNodeBySecret(secret)
	if node == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}

	disguise := node.DisguiseName
	if disguise == "" {
		disguise = "gost-node-lite"
	}

	panelAddr := installPanelAddress(c)
	useTLS := strings.HasPrefix(panelAddr, "https://")

	addrValue := panelAddr
	addrValue = strings.TrimPrefix(addrValue, "http://")
	addrValue = strings.TrimPrefix(addrValue, "https://")
	addrValue = strings.TrimSuffix(addrValue, "/")

	script := fmt.Sprintf(`#!/bin/sh
set -e

DISGUISE=%s
NODE_SECRET=%s
PANEL_ADDR=%s
ADDR_VALUE=%s
USE_TLS=%t

BIN_PATH="/usr/local/bin/$DISGUISE"
CFG_DIR="/etc/$DISGUISE"
UNIT_PATH="/etc/systemd/system/$DISGUISE.service"
INIT_PATH="/etc/init.d/$DISGUISE"

if [ "$(id -u)" != "0" ]; then
    echo "Please run as root."
    exit 1
fi

CURL_FLAGS="-fsSL"
if [ "$1" = "6" ]; then
    CURL_FLAGS="-6fsSL"
    echo "IPv6 download mode enabled."
fi

RAW_ARCH=$(uname -m)
case "$RAW_ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    armv7l|armv7*|armhf) ARCH="arm" ;;
    *) echo "Unsupported architecture: $RAW_ARCH"; exit 1 ;;
esac

SERVICE_MANAGER=""
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    SERVICE_MANAGER="systemd"
elif command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1; then
    SERVICE_MANAGER="openrc"
else
    echo "Unsupported service manager. This lite installer supports systemd and OpenRC."
    exit 1
fi

download() {
    url="$1"
    dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl $CURL_FLAGS "$url" -o "$dest"
    elif command -v wget >/dev/null 2>&1; then
        if [ "$CURL_FLAGS" = "-6fsSL" ]; then
            echo "wget-only systems cannot force IPv6; downloading with default routing."
        fi
        wget -qO "$dest" "$url"
    else
        echo "curl or wget is required."
        exit 1
    fi
}

is_systemd_managed() {
    [ -f "$UNIT_PATH" ] && grep -Fq "ExecStart=$BIN_PATH" "$UNIT_PATH" && grep -Fq "WorkingDirectory=$CFG_DIR" "$UNIT_PATH"
}

is_openrc_managed() {
    [ -f "$INIT_PATH" ] && grep -Fq "command=\"$BIN_PATH\"" "$INIT_PATH" && grep -Fq "directory=\"$CFG_DIR\"" "$INIT_PATH"
}

if [ -f "$UNIT_PATH" ] && ! is_systemd_managed; then
    echo "Service $DISGUISE already exists and is not managed by this installer. Aborting."
    exit 1
fi

if [ -f "$INIT_PATH" ] && ! is_openrc_managed; then
    echo "OpenRC service $DISGUISE already exists and is not managed by this installer. Aborting."
    exit 1
fi

if [ -e "$BIN_PATH" ] && [ ! -f "$UNIT_PATH" ] && [ ! -f "$INIT_PATH" ]; then
    echo "Binary $BIN_PATH already exists without a managed service. Aborting."
    exit 1
fi

if [ "$SERVICE_MANAGER" = "systemd" ] && is_systemd_managed; then
    echo "Stopping existing $DISGUISE service..."
    systemctl stop "$DISGUISE" 2>/dev/null || true
elif [ "$SERVICE_MANAGER" = "openrc" ] && is_openrc_managed; then
    echo "Stopping existing $DISGUISE service..."
    rc-service "$DISGUISE" stop 2>/dev/null || true
fi

echo "Downloading lite node agent for $ARCH..."
TMP_BIN="$BIN_PATH.tmp"
rm -f "$TMP_BIN"
download "$PANEL_ADDR/s/$NODE_SECRET/b/$ARCH" "$TMP_BIN"
chmod +x "$TMP_BIN"
mv "$TMP_BIN" "$BIN_PATH"

mkdir -p "$CFG_DIR"

cat > "$CFG_DIR/config.json" <<EOF
{
  "addr": "$ADDR_VALUE",
  "secret": "$NODE_SECRET",
  "use_tls": $USE_TLS,
  "v_bin": "/bin/true",
  "v_cfg": "service.json"
}
EOF

if [ ! -f "$CFG_DIR/runtime.json" ]; then
    echo "{}" > "$CFG_DIR/runtime.json"
fi

cat > "$CFG_DIR/uninstall.sh" <<EOF
#!/bin/sh
set -e
DISGUISE="$DISGUISE"
BIN_PATH="$BIN_PATH"
CFG_DIR="$CFG_DIR"
UNIT_PATH="$UNIT_PATH"
INIT_PATH="$INIT_PATH"

if command -v systemctl >/dev/null 2>&1 && [ -f "$UNIT_PATH" ]; then
    systemctl stop "$DISGUISE" 2>/dev/null || true
    systemctl disable "$DISGUISE" 2>/dev/null || true
    rm -f "$UNIT_PATH"
    systemctl daemon-reload 2>/dev/null || true
fi

if command -v rc-service >/dev/null 2>&1 && [ -f "$INIT_PATH" ]; then
    rc-service "$DISGUISE" stop 2>/dev/null || true
    rc-update del "$DISGUISE" default 2>/dev/null || true
    rm -f "$INIT_PATH"
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop "$DISGUISE-audit.timer" 2>/dev/null || true
    systemctl disable "$DISGUISE-audit.timer" 2>/dev/null || true
    rm -f "/etc/systemd/system/$DISGUISE-audit.timer" "/etc/systemd/system/$DISGUISE-audit.service"
    systemctl daemon-reload 2>/dev/null || true
fi
[ -f /etc/crontabs/root ] && sed -i "/$DISGUISE-audit/d" /etc/crontabs/root 2>/dev/null || true
rm -f "/usr/local/bin/$DISGUISE-audit"

rm -f "$BIN_PATH"
rm -rf "$CFG_DIR"
echo "Lite node uninstalled."
EOF
chmod +x "$CFG_DIR/uninstall.sh"

if [ "$SERVICE_MANAGER" = "systemd" ]; then
    cat > "$UNIT_PATH" <<EOF
[Unit]
Description=$DISGUISE daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$CFG_DIR
ExecStart=$BIN_PATH
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$DISGUISE"
    systemctl restart "$DISGUISE"
    echo "Logs: journalctl -u $DISGUISE -f"
else
    cat > "$INIT_PATH" <<EOF
#!/sbin/openrc-run
name="$DISGUISE"
description="$DISGUISE daemon"
command="$BIN_PATH"
directory="$CFG_DIR"
pidfile="/run/$DISGUISE.pid"
supervisor="supervise-daemon"
respawn_delay=3
respawn_max=0
output_log="/var/log/$DISGUISE.log"
error_log="/var/log/$DISGUISE.err"

depend() {
    need net
}
EOF
    chmod +x "$INIT_PATH"
    rc-update add "$DISGUISE" default
    rc-service "$DISGUISE" restart
    echo "Logs: tail -f /var/log/$DISGUISE.log"
fi

# --- sing-box audit-log watchdog: auto-mounts access.log so panel audit works ---
AUDIT_BIN="/usr/local/bin/$DISGUISE-audit"
if download "$PANEL_ADDR/s/$NODE_SECRET/audit-watchdog" "$AUDIT_BIN.tmp"; then
    chmod +x "$AUDIT_BIN.tmp"
    mv "$AUDIT_BIN.tmp" "$AUDIT_BIN"
    if [ "$SERVICE_MANAGER" = "systemd" ]; then
        cat > "/etc/systemd/system/$DISGUISE-audit.service" <<EOF
[Unit]
Description=$DISGUISE audit helper

[Service]
Type=oneshot
ExecStart=$AUDIT_BIN
EOF
        cat > "/etc/systemd/system/$DISGUISE-audit.timer" <<EOF
[Unit]
Description=$DISGUISE audit helper timer

[Timer]
OnBootSec=2min
OnUnitActiveSec=5min

[Install]
WantedBy=timers.target
EOF
        systemctl daemon-reload
        systemctl enable "$DISGUISE-audit.timer" 2>/dev/null || true
        systemctl start "$DISGUISE-audit.timer" 2>/dev/null || true
    else
        mkdir -p /etc/crontabs
        if ! grep -q "$DISGUISE-audit" /etc/crontabs/root 2>/dev/null; then
            echo "*/5 * * * * $AUDIT_BIN" >> /etc/crontabs/root
        fi
        rc-update add crond default 2>/dev/null || true
        (rc-service crond restart 2>/dev/null || rc-service crond start 2>/dev/null) || true
    fi
    "$AUDIT_BIN" 2>/dev/null || true
    echo "Audit watchdog installed."
fi

echo "Lite node installed and started successfully."
echo "Config: $CFG_DIR/config.json"
echo "Uninstall: sh $CFG_DIR/uninstall.sh"
`, scriptShellQuote(disguise), scriptShellQuote(node.Secret), scriptShellQuote(panelAddr), scriptShellQuote(addrValue), useTLS)

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, script)
}

// CamoAuditWatchdogScript serves the sing-box audit-log auto-mount watchdog.
// The script is static (no per-node data) but served under the camouflaged
// secret path so it validates against a real node and matches the install flow.
func CamoAuditWatchdogScript(c *gin.Context) {
	secret := c.Param("secret")
	if findNodeBySecret(secret) == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, auditWatchdogScript)
}

// auditWatchdogScript keeps the running sing-box writing its access log to the
// fixed path the node agent's audit tailer reads, so the panel audit column
// stays "active". It is idempotent, validates before restarting, and rolls back
// on a failed config check. Must contain no backtick characters (Go raw string).
const auditWatchdogScript = `#!/bin/sh
# Auto-mount the sing-box access log for node audit. Idempotent; safe to re-run.
LOG_DIR="/var/log/sing-box"
LOG_PATH="$LOG_DIR/access.log"

# Locate the running sing-box proxy: a process whose binary is actually sing-box
# (matching cmdline text alone would also catch this very script).
PID=""
for cand in $(pgrep -f sing-box 2>/dev/null); do
    [ "$cand" = "$$" ] && continue
    cbin=$(tr '\0' '\n' < "/proc/$cand/cmdline" 2>/dev/null | head -1)
    case "$cbin" in
        */sing-box|sing-box) PID="$cand"; break ;;
    esac
done
[ -z "$PID" ] && exit 0

CMDFILE="/proc/$PID/cmdline"
[ -r "$CMDFILE" ] || exit 0

BIN=""
CFGS=""
CONFDIR=""
prev=""
first=1
for tok in $(tr '\0' '\n' < "$CMDFILE"); do
    if [ "$first" = "1" ]; then BIN="$tok"; first=0; fi
    case "$prev" in
        -c) CFGS="$CFGS $tok" ;;
        -C) CONFDIR="$tok" ;;
    esac
    prev="$tok"
done
[ -n "$BIN" ] || BIN="sing-box"
[ -n "$CFGS" ] || exit 0

# Already writing to our path anywhere? Nothing to do.
if grep -rqsF "$LOG_PATH" $CFGS $CONFDIR 2>/dev/null; then
    exit 0
fi

SBUSER=$(stat -c '%U' "/proc/$PID" 2>/dev/null)
[ -n "$SBUSER" ] || SBUSER="root"

mkdir -p "$LOG_DIR"
chown "$SBUSER:$SBUSER" "$LOG_DIR" 2>/dev/null || true
chmod 755 "$LOG_DIR" 2>/dev/null || true

# Inject the log block: prefer the merge dir (does not touch a managed config.json).
INJECTED=""
if [ -n "$CONFDIR" ] && [ -d "$CONFDIR" ]; then
    F="$CONFDIR/zz-flux-audit-log.json"
    printf '{"log":{"output":"%s","timestamp":true}}\n' "$LOG_PATH" > "$F"
    chown "root:$SBUSER" "$F" 2>/dev/null || true
    chmod 640 "$F" 2>/dev/null || true
    INJECTED="file:$F"
else
    command -v jq >/dev/null 2>&1 || exit 0
    MAINCFG=""
    for f in $CFGS; do MAINCFG="$f"; break; done
    [ -n "$MAINCFG" ] || exit 0
    [ -f "$MAINCFG.flux-audit-bak" ] || cp -p "$MAINCFG" "$MAINCFG.flux-audit-bak" 2>/dev/null || true
    TMP="$MAINCFG.flux-tmp"
    if jq --arg p "$LOG_PATH" '.log = ((.log // {}) + {output:$p, timestamp:true})' "$MAINCFG" > "$TMP" 2>/dev/null; then
        mv "$TMP" "$MAINCFG"
        INJECTED="cfg:$MAINCFG"
    else
        rm -f "$TMP"
        exit 0
    fi
fi

# Validate the merged config before restarting; roll back if it fails.
CHECK_ARGS=""
for f in $CFGS; do CHECK_ARGS="$CHECK_ARGS -c $f"; done
[ -n "$CONFDIR" ] && CHECK_ARGS="$CHECK_ARGS -C $CONFDIR"
if ! env ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true ENABLE_DEPRECATED_OUTBOUND_DNS_RULE_ITEM=true ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true "$BIN" check $CHECK_ARGS >/dev/null 2>&1; then
    case "$INJECTED" in
        file:*) rm -f "${INJECTED#file:}" ;;
        cfg:*) CF="${INJECTED#cfg:}"; [ -f "$CF.flux-audit-bak" ] && cp -p "$CF.flux-audit-bak" "$CF" ;;
    esac
    exit 0
fi

# Restart the sing-box service so the new log config takes effect.
restarted=""
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    UNIT=$(sed -n 's#.*/\([A-Za-z0-9@._-]*\.service\)$#\1#p' "/proc/$PID/cgroup" 2>/dev/null | tail -1)
    if [ -n "$UNIT" ] && systemctl restart "$UNIT" 2>/dev/null; then restarted=1; fi
    if [ -z "$restarted" ]; then
        for u in sing-box sb-sing-box; do systemctl restart "$u" 2>/dev/null && { restarted=1; break; }; done
    fi
fi
if [ -z "$restarted" ] && command -v rc-service >/dev/null 2>&1; then
    for s in sing-box sb-sing-box; do rc-service "$s" restart 2>/dev/null && { restarted=1; break; }; done
fi
if [ -z "$restarted" ]; then
    kill "$PID" 2>/dev/null || true
fi
exit 0
`

// CamoInstallBinary serves the gost-node binary via camouflaged URL.
func CamoInstallBinary(c *gin.Context) {
	secret := c.Param("secret")
	node := findNodeBySecret(secret)
	if node == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}

	arch := c.Param("arch")
	if !allowedArchs[arch] {
		c.String(http.StatusBadRequest, "invalid architecture")
		return
	}

	binaryPath := filepath.Join(config.Cfg.NodeBinaryDir, fmt.Sprintf("node-%s", arch))
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "binary not found")
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=bin-%s", arch))
	c.File(binaryPath)
}

// CamoInstallXray serves the xray binary via camouflaged URL.
func CamoInstallXray(c *gin.Context) {
	secret := c.Param("secret")
	node := findNodeBySecret(secret)
	if node == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}

	arch := c.Param("arch")
	if !allowedArchs[arch] {
		c.String(http.StatusBadRequest, "invalid architecture")
		return
	}

	binaryPath := filepath.Join(config.Cfg.NodeBinaryDir, fmt.Sprintf("xray-%s", arch))
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		c.String(http.StatusNotFound, "binary not found")
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=svc-%s", arch))
	c.File(binaryPath)
}
