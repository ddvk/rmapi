#!/bin/sh
# Simple entrypoint - ensure directories exist and run as app user
mkdir -p /home/app/.config/rmapi /home/app/.cache/rmapi /home/app/downloads
exec su-exec app "$@"
