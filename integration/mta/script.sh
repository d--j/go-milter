
die() {
  while [ $# -gt 0 ]; do
    echo "$1"
    shift
  done
  exit 1
}

usage() {
  die "$@" "Usage: $0 [tags|start|stop]"
}

parse_args() {
  shift
  while [ $# -gt 0 ]; do
    case $1 in
    -mtaPort)
      MTA_PORT="$2"
      shift
      shift
      ;;
    -milterPort)
      MILTER_PORT="$2"
      shift
      shift
      ;;
    -receiverPort)
      RECEIVER_PORT="$2"
      shift
      shift
      ;;
    -scratchDir)
      SCRATCH_DIR="$2"
      shift
      shift
      ;;
    *)
      usage "unknown argument $1"
      ;;
    esac
  done
  if [ -z "$MTA_PORT" ] || [ -z "$MILTER_PORT" ] || [ -z "$RECEIVER_PORT" ] || [ -z "$SCRATCH_DIR" ]; then
    usage "missing required arguments"
  fi
  export MTA_PORT MILTER_PORT RECEIVER_PORT SCRATCH_DIR
}

render_template() {
  awk '{while(match($0,"[%]{[^}]*}")) {var=substr($0,RSTART+2,RLENGTH -3);gsub("[%]{"var"}",ENVIRON[var])}}1'
}
