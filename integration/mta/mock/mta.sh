#!/usr/bin/env sh

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$SCRIPT_DIR/../script.sh"

if [ -z "$1" ]; then usage; fi

if [ "tags" = "$1" ]; then
  echo "exec-foreground"
  echo "mta-mock"
  echo "auth-no"
  echo "auth-plain"
  echo "tls-no"
  echo "tls-starttls"
  exit 0
fi

if [ "start" = "$1" ]; then
  parse_args "$@"
  go build -o "$SCRATCH_DIR/mta.exe" -v "$SCRIPT_DIR"
  exec "$SCRATCH_DIR/mta.exe" -mta ":$MTA_PORT" -next ":$RECEIVER_PORT" -milter ":$MILTER_PORT" -cert "$SCRATCH_DIR/../cert.pem" -key "$SCRATCH_DIR/../key.pem"
fi

if [ "stop" = "$1" ]; then
  parse_args "$@"
  exit 0
fi

usage "Unknown command $1"
