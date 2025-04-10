#!/usr/bin/env sh

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$SCRIPT_DIR/../script.sh"

if [ -z "$1" ]; then usage; fi

if [ "tags" = "$1" ]; then
  if ! command -v postfix >/dev/null; then
    die "no postfix executable found"
  fi
  echo "exec-foreground"
  echo "mta-postfix"
  echo "auth-no"
  if [ -z "$SKIP_POSTFIX_AUTH" ]; then
    echo "auth-plain"
  fi
  echo "tls-no"
  echo "tls-starttls"
  exit 0
fi

setup_chroot() {
  POSTCONF="postconf -o inet_interfaces= -c $SCRATCH_DIR/conf"
  # Make sure that the chroot environment is set up correctly.
  umask 022
  queue_dir=$($POSTCONF -hx queue_directory)
  cd "$queue_dir" || die "cannot cd into queue_dir"

  # copy the smtp CA path if specified
  sca_path=$($POSTCONF -hx smtp_tls_CApath)
  case "$sca_path" in
  '') : ;;           # no sca_path
  $queue_dir/*) : ;; # skip stuff already in chroot
  *)
    if test -d "$sca_path"; then
      dest_dir="$queue_dir/${sca_path#/}"
      # strip any/all trailing /
      while [ "${dest_dir%/}" != "${dest_dir}" ]; do
        dest_dir="${dest_dir%/}"
      done
      new=0
      if test -d "$dest_dir"; then
        # write to a new directory ...
        dest_dir="${dest_dir}.NEW"
        new=1
      fi
      mkdir --parent ${dest_dir}
      # handle files in subdirectories
      (cd "$sca_path" && find . -name '*.pem' -not -xtype l -print0 | cpio -0pdL --quiet "$dest_dir") 2>/dev/null ||
        (
          echo failure copying certificates
          exit 1
        )
      c_rehash "$dest_dir" >/dev/null 2>&1
      if [ "$new" = 1 ]; then
        # and replace the old directory
        rm -rf "${dest_dir%.NEW}"
        mv "$dest_dir" "${dest_dir%.NEW}"
      fi
    fi
    ;;
  esac

  # copy the smtpd CA path if specified
  dca_path=$($POSTCONF -hx smtpd_tls_CApath)
  case "$dca_path" in
  '') : ;;           # no dca_path
  $queue_dir/*) : ;; # skip stuff already in chroot
  *)
    if test -d "$dca_path"; then
      dest_dir="$queue_dir/${dca_path#/}"
      # strip any/all trailing /
      while [ "${dest_dir%/}" != "${dest_dir}" ]; do
        dest_dir="${dest_dir%/}"
      done
      new=0
      if test -d "$dest_dir"; then
        # write to a new directory ...
        dest_dir="${dest_dir}.NEW"
        new=1
      fi
      mkdir --parent ${dest_dir}
      # handle files in subdirectories
      (cd "$dca_path" && find . -name '*.pem' -not -xtype l -print0 | cpio -0pdL --quiet "$dest_dir") 2>/dev/null ||
        (
          echo failure copying certificates
          exit 1
        )
      c_rehash "$dest_dir" >/dev/null 2>&1
      if [ "$new" = 1 ]; then
        # and replace the old directory
        rm -rf "${dest_dir%.NEW}"
        mv "$dest_dir" "${dest_dir%.NEW}"
      fi
    fi
    ;;
  esac

  # if we're using unix:passwd.byname, then we need to add etc/passwd.
  local_maps=$($POSTCONF -hx local_recipient_maps)
  if [ "X$local_maps" != "X${local_maps#*unix:passwd.byname}" ]; then
    if [ "X$local_maps" = "X${local_maps#*proxy:unix:passwd.byname}" ]; then
      sed 's/^\([^:]*\):[^:]*/\1:x/' /etc/passwd >etc/passwd
      chmod a+r etc/passwd
    fi
  fi

  FILES="etc/localtime etc/services etc/resolv.conf etc/hosts \
  	   etc/host.conf etc/nsswitch.conf etc/nss_mdns.config etc/sasldb2"
  for file in $FILES; do
    [ -d ${file%/*} ] || mkdir -p ${file%/*}
    if [ -f /${file} ]; then rm -f ${file} && cp /${file} ${file}; fi
    if [ -f ${file} ]; then chmod a+rX ${file}; fi
  done
  # ldaps needs this. debian bug 572841
  (
    echo /dev/random
    echo /dev/urandom
  ) | cpio -pdL --quiet . 2>/dev/null || true
  rm -f usr/lib/zoneinfo/localtime
  mkdir -p usr/lib/zoneinfo
  ln -sf /etc/localtime usr/lib/zoneinfo/localtime

  LIBLIST=$(for name in gcc_s nss resolv; do
    for f in /lib/*/lib${name}*.so* /lib/lib${name}*.so*; do
      if [ -f "$f" ]; then echo ${f#/}; fi
    done
  done)

  if [ -n "$LIBLIST" ]; then
    for f in $LIBLIST; do
      rm -f "$f"
    done
    tar cf - -C / $LIBLIST 2>/dev/null | tar xf -
  fi
}

if [ "start" = "$1" ]; then
  parse_args "$@"
  mkdir "$SCRATCH_DIR/conf" "$SCRATCH_DIR/conf/sasl" "$SCRATCH_DIR/data" "$SCRATCH_DIR/queue" || die "could not create $SCRATCH_DIR/{conf,data,queue}"
  render_template <"$SCRIPT_DIR/main.cf" >"$SCRATCH_DIR/conf/main.cf" || die "could not create $SCRATCH_DIR/conf/main.cf"
  render_template <"$SCRIPT_DIR/master.cf" >"$SCRATCH_DIR/conf/master.cf" || die "could not create $SCRATCH_DIR/conf/master.cf"
  cp "$SCRIPT_DIR/dhparam.pem" "$SCRATCH_DIR/conf/dhparam.pem" || die "could not create $SCRATCH_DIR/conf/dhparam.pem"
  cp "$SCRATCH_DIR/../cert.pem" "$SCRATCH_DIR/conf/cert.pem" || die "could not create $SCRATCH_DIR/conf/cert.pem"
  cp "$SCRATCH_DIR/../key.pem" "$SCRATCH_DIR/conf/key.pem" || die "could not create $SCRATCH_DIR/conf/key.pem"
  render_template <"$SCRIPT_DIR/smtpd.conf" >"$SCRATCH_DIR/conf/sasl/smtpd.conf" || die "could not create $SCRATCH_DIR/conf/sasl/smtpd.conf"
  sudo -n -- chown -R postfix:postfix "$SCRATCH_DIR/data" || die "could not chown $SCRATCH_DIR/data"
  sudo -n -- postfix -v -c "$SCRATCH_DIR/conf" check || die "postfix config check failed"
  echo "password1" | sudo -n -- saslpasswd2 -c -p -u example.com user1 || die "cannot create SASL user user1"
  echo "password2" | sudo -n -- saslpasswd2 -c -p -u example.com user2 || die "cannot create SASL user user2"
  setup_chroot
  sudo -n -- postfix -v -c "$SCRATCH_DIR/conf" start-fg
  exit 0
fi

if [ "stop" = "$1" ]; then
  parse_args "$@"
  sudo -n -- postfix -v -c "$SCRATCH_DIR/conf" stop
  exit 0
fi

usage "Unknown command $1"
