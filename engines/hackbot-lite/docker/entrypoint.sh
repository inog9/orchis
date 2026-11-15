#!/bin/sh
set -eu
umask 077
mkdir -p "$HOME/.pi/agent" /data
if [ ! -e "$HOME/.pi/agent/node_modules" ]; then
  ln -s /opt/pi-packages/node_modules "$HOME/.pi/agent/node_modules"
fi
# Seed runtime packages only when the persistent home has no settings yet.
if [ ! -f "$HOME/.pi/agent/settings.json" ]; then
  cp /opt/pentest/pi-settings.json "$HOME/.pi/agent/settings.json"
fi
# Bootstrap gog in the persistent home before every command, including service
# jobs. Python execs the requested argv so signals and exit codes stay intact.
exec python3 /opt/pentest/docker/gog-bootstrap.py -- "$@"
