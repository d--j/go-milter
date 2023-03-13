#!/usr/bin/env sh

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$SCRIPT_DIR/../script.sh"

if [ -z "$1" ]; then usage; fi

if [ "tags" = "$1" ]; then
  if [ ! -x /usr/libexec/sendmail/sendmail ]; then
    die "sendmail not installed"
  fi
  #echo "exec-foreground"
  echo "mta-sendmail"
  echo "auth-no"
  #echo "auth-plain"
  echo "tls-no"
  echo "tls-starttls"
  exit 0
fi

if [ "start" = "$1" ]; then
  parse_args "$@"
  cp "$SCRATCH_DIR/../cert.pem" "$SCRATCH_DIR/cert.pem" || die "could not create $SCRATCH_DIR/cert.pem"
  cp "$SCRATCH_DIR/../key.pem" "$SCRATCH_DIR/key.pem" || die "could not create $SCRATCH_DIR/key.pem"
  render_template <"$SCRIPT_DIR/sendmail.cf" >"$SCRATCH_DIR/sendmail.cf" || die "could not create $SCRATCH_DIR/sendmail.cf"
  mkdir "${SCRATCH_DIR}/mqueue" || die "could not create $SCRATCH_DIR/mqueue"
  sudo -n -- chown smmta:smmsp "${SCRATCH_DIR}/mqueue" || die "could not chown $SCRATCH_DIR/mqueue"
  sudo -n -- chmod u=rwx,g=rs,o= "${SCRATCH_DIR}/mqueue" || die "could not chmod $SCRATCH_DIR/mqueue"
  sudo -n -- syslog-ng --no-caps || die "could not start syslog-ng"
  sudo -n -- /usr/libexec/sendmail/sendmail -bd "-C${SCRATCH_DIR}/sendmail.cf"
  exit 0
fi

if [ "stop" = "$1" ]; then
  parse_args "$@"
  # echo "SHUTDOWN" > "${SCRATCH_DIR}/smcontrol"
  kill "$(head -n1 "${SCRATCH_DIR}/sendmail.pid")"
  sleep 2
  cat /var/log/messages
  exit 0
fi

usage "Unknown command $1"
