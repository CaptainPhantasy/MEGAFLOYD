#!/bin/bash
# Telemetry tick: keeps the local trace store alive and folds new session
# activity into it. Started by launchd; safe to run by hand.
set -u

ROOT="/Volumes/SanDisk1Tb/MEGAFLOYD"
BIN="$ROOT/bin/megafloyd"
JAEGER="$HOME/.local/libexec/jaeger/jaeger"
CONFIG="$HOME/.megafloyd/telemetry/jaeger.yaml"
LOG_DIR="$HOME/.megafloyd/logs"
LOG="$LOG_DIR/telemetry-tick.log"

mkdir -p "$LOG_DIR" "$HOME/.megafloyd/telemetry/jaeger/keys" "$HOME/.megafloyd/telemetry/jaeger/values"

if ! pgrep -f "libexec/jaeger/jaeger --config" >/dev/null 2>&1; then
	nohup "$JAEGER" --config "$CONFIG" >>"$LOG_DIR/jaeger.log" 2>&1 &
	sleep 3
fi

{
	printf '[%s] tick\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	"$BIN" telemetry collect
	"$BIN" telemetry ship
} >>"$LOG" 2>&1
