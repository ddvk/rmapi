#!/bin/sh
# Don't use set -e here as we need to capture exit codes properly
set +e

# Ensure directories exist
mkdir -p /home/app/.config/rmapi /home/app/.cache/rmapi /home/app/downloads

# Backup rmapi.conf function
# This creates a timestamped backup of rmapi.conf containing authentication tokens
# The backup is stored in ./io/config/backup/ (mounted volume) for local access
backup_rmapi_conf() {
    CONFIG_FILE="/home/app/.config/rmapi/rmapi.conf"
    BACKUP_DIR="/home/app/.config/rmapi/backup"
    
    if [ -f "$CONFIG_FILE" ]; then
        mkdir -p "$BACKUP_DIR"
        BACKUP_FILE="$BACKUP_DIR/rmapi.conf.$(date +%Y%m%d_%H%M%S)"
        cp "$CONFIG_FILE" "$BACKUP_FILE" 2>/dev/null || true
        # Keep only the latest 5 backups (remove older ones)
        ls -t "$BACKUP_DIR"/rmapi.conf.* 2>/dev/null | tail -n +6 | xargs rm -f 2>/dev/null || true
    fi
}

# Backup existing config file if present (before command execution)
backup_rmapi_conf

# Change app user UID/GID to match host for file sharing between containers
if [ "$(id -u)" = "0" ] && [ -n "${HOST_UID}" ] && [ -n "${HOST_GID}" ]; then
    CURRENT_UID=$(id -u app 2>/dev/null || echo "1000")
    CURRENT_GID=$(id -g app 2>/dev/null || echo "1000")
    
    # Update group file
    [ "${CURRENT_GID}" != "${HOST_GID}" ] && sed -i "s/^app:\([^:]*\):${CURRENT_GID}:/app:\1:${HOST_GID}:/" /etc/group
    
    # Update passwd file (UID and GID)
    sed -i "s/^app:\([^:]*\):${CURRENT_UID}:\([^:]*\):/app:\1:${HOST_UID}:${HOST_GID}:/" /etc/passwd
    
    # Update file ownership
    [ "${CURRENT_GID}" != "${HOST_GID}" ] && find /home/app -group "${CURRENT_GID}" -exec chgrp "${HOST_GID}" {} + 2>/dev/null || true
    [ "${CURRENT_UID}" != "${HOST_UID}" ] && find /home/app -user "${CURRENT_UID}" -exec chown "${HOST_UID}" {} + 2>/dev/null || true
    
    # Set permissions on mounted volumes
    chown -R "${HOST_UID}:${HOST_GID}" /home/app/.config/rmapi /home/app/.cache/rmapi /home/app/downloads 2>/dev/null || true
    chmod -R u+rwX /home/app/.config/rmapi /home/app/.cache/rmapi /home/app/downloads 2>/dev/null || true
    
    # Execute command and backup config after completion
    # Note: We don't use exec here so backup can run after command completes
    su-exec app "$@"
    EXIT_CODE=$?
    backup_rmapi_conf
    exit $EXIT_CODE
else
    chmod -R u+rwX /home/app/.config/rmapi /home/app/.cache/rmapi /home/app/downloads 2>/dev/null || true
    # Execute command and backup config after completion
    "$@"
    EXIT_CODE=$?
    backup_rmapi_conf
    exit $EXIT_CODE
fi

