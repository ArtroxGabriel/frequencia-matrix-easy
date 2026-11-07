#!/usr/bin/env bash
# Brings up the local stack (Synapse, Element Web and the app in real mode) and
# prepares a classroom to try it: a teacher, three students, a room called SD
# with the widget added, and the room PIN for clients without OpenID.
#
# It can be run again at any time: whatever exists is reused.
#
#   ./scripts/dev-stack.sh              start and prepare everything
#   ./scripts/dev-stack.sh clean        ERASE everything, then recreate it from scratch
#   ./scripts/dev-stack.sh clean --yes  the same, without asking for confirmation
#   ./scripts/dev-stack.sh --new-pin    also generate a new room PIN
#   ./scripts/dev-stack.sh --no-tailscale
#
# "clean" removes the containers, the Synapse and app volumes (every account,
# room and attendance record), the app image and .dev-stack/. It only touches
# this project's Docker resources.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

HS_URL=${HS_URL:-http://localhost:8008}
APP_LOCAL_URL=http://localhost:8080
SERVER_NAME=localhost
ROOM_NAME=SD
WIDGET_ID=frequencia
# Names that older test accounts are unlikely to use: an account that already
# exists with another password cannot be reused.
BOT=${BOT:-frequenciabot}
TEACHER=${TEACHER:-professor-sd}
STUDENTS=(aluno1-sd aluno2-sd aluno3-sd)
STATE_DIR=.dev-stack
STATE_FILE=$STATE_DIR/state.env

USE_TAILSCALE=1
NEW_PIN=0
CLEAN=0
ASSUME_YES=0

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33maviso:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merro:\033[0m %s\n' "$*" >&2; exit 1; }

usage() { awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "${BASH_SOURCE[0]}"; }

for arg in "$@"; do
  case $arg in
    --no-tailscale) USE_TAILSCALE=0 ;;
    --new-pin) NEW_PIN=1 ;;
    clean|--clean|--reset) CLEAN=1 ;;
    -y|--yes) ASSUME_YES=1 ;;
    -h|--help) usage; exit 0 ;;
    *) die "opção desconhecida: $arg (veja --help)" ;;
  esac
done

for tool in docker curl jq; do
  command -v "$tool" >/dev/null || die "preciso do '$tool' instalado"
done

password_of() { printf 'senha-%s-123' "$1"; }
mxid_of()     { printf '@%s:%s' "$1" "$SERVER_NAME"; }
urlencode()   { jq -rn --arg v "$1" '$v|@uri'; }

state_get() { [[ -f $STATE_FILE ]] && sed -n "s/^$1=//p" "$STATE_FILE" | tail -1 || true; }
state_set() {
  mkdir -p "$STATE_DIR"
  touch "$STATE_FILE"
  chmod 600 "$STATE_FILE"
  { grep -v "^$1=" "$STATE_FILE" || true; echo "$1=$2"; } > "$STATE_FILE.tmp"
  mv "$STATE_FILE.tmp" "$STATE_FILE"
}

# api METHOD TOKEN PATH [JSON]: calls the Synapse client API, prints the body.
api() {
  local method=$1 token=$2 path=$3 body=${4:-}
  local args=(-sS -X "$method" -H 'Content-Type: application/json')
  [[ -n $token ]] && args+=(-H "Authorization: Bearer $token")
  [[ -n $body ]] && args+=(-d "$body")
  curl "${args[@]}" "$HS_URL$path"
}

# api_ok: like api, but stops when Synapse answers with an error.
api_ok() {
  local out
  out=$(api "$@")
  if jq -e '.errcode' <<<"$out" >/dev/null 2>&1; then
    die "o Synapse recusou $2 $3: $out"
  fi
  printf '%s' "$out"
}

wait_for() { # URL SECONDS LABEL
  local i
  for ((i = 0; i < $2; i++)); do
    curl -fsS -o /dev/null "$1" 2>/dev/null && return 0
    sleep 1
  done
  die "$3 não respondeu em $2 segundos ($1)"
}

# ensure_account USER: registers the account, or logs in when it already exists.
# Prints the access token.
ensure_account() {
  local user=$1 pass resp
  pass=$(password_of "$user")
  resp=$(api POST '' /_matrix/client/v3/register \
    "$(jq -nc --arg u "$user" --arg p "$pass" '{username:$u,password:$p,auth:{type:"m.login.dummy"}}')")
  if jq -e '.access_token' <<<"$resp" >/dev/null 2>&1; then
    jq -r '.access_token' <<<"$resp"
    return
  fi
  [[ $(jq -r '.errcode // ""' <<<"$resp") == M_USER_IN_USE ]] ||
    die "não consegui criar @$user: $resp"
  resp=$(api POST '' /_matrix/client/v3/login \
    "$(jq -nc --arg u "$user" --arg p "$pass" '{type:"m.login.password",identifier:{type:"m.id.user",user:$u},password:$p}')")
  jq -e '.access_token' <<<"$resp" >/dev/null 2>&1 ||
    die "@$user já existe com outra senha. Use --reset para recomeçar do zero (apaga tudo)."
  jq -r '.access_token' <<<"$resp"
}

set_display_name() { # USER TOKEN NAME
  api_ok PUT "$2" "/_matrix/client/v3/profile/$(urlencode "$(mxid_of "$1")")/displayname" \
    "$(jq -nc --arg n "$3" '{displayname:$n}')" >/dev/null
}

joined_rooms() { api_ok GET "$1" /_matrix/client/v3/joined_rooms | jq -r '.joined_rooms[]'; }

has_joined() { # TOKEN ROOM_ID
  local rooms
  rooms=$(joined_rooms "$1")
  grep -qxF "$2" <<<"$rooms" # not a pipe: grep -q would break it under pipefail
}

# find_room TOKEN: the id of the teacher's room called SD, if there is one.
find_room() {
  local room name
  while read -r room; do
    [[ -n $room ]] || continue
    name=$(api GET "$1" "/_matrix/client/v3/rooms/$(urlencode "$room")/state/m.room.name/" | jq -r '.name // ""')
    if [[ $name == "$ROOM_NAME" ]]; then
      printf '%s' "$room"
      return
    fi
  done < <(joined_rooms "$1")
}

clean_everything() {
  warn "clean APAGA todas as contas, salas e dados do Synapse e do app deste projeto, e a imagem do app."
  if [[ $ASSUME_YES != 1 ]]; then
    [[ -t 0 ]] || die "sem terminal para confirmar. Rode com --yes para confirmar."
    read -r -p "Digite 'apagar' para continuar: " answer
    [[ $answer == apagar ]] || die "cancelado, nada foi apagado"
  fi
  log "Removendo containers, volumes e a imagem do app"
  docker compose -f docker-compose.yml -f docker-compose.real.yml \
    down --volumes --remove-orphans --rmi local
  rm -rf "$STATE_DIR"
  log "Ambiente limpo, recriando do zero"
}

setup_tailscale() {
  command -v tailscale >/dev/null || { warn "tailscale não está instalado"; return 1; }
  local status
  status=$(tailscale status --json 2>/dev/null) || { warn "não consegui consultar o tailscale"; return 1; }
  [[ $(jq -r '.BackendState' <<<"$status") == Running ]] || { warn "o tailscale não está conectado (tailscale up)"; return 1; }
  TS_HOST=$(jq -r '.Self.DNSName | sub("\\.$"; "")' <<<"$status")
  local port target
  for entry in 443:8080 8443:8008 10000:8081; do
    port=${entry%%:*}; target=${entry##*:}
    tailscale serve --bg --https="$port" "http://localhost:$target" >/dev/null 2>&1 ||
      { warn "tailscale serve falhou na porta $port (o Serve está habilitado na sua tailnet?)"; return 1; }
  done
}

start_synapse_and_element() {
  log "Subindo Synapse e Element Web"
  docker compose up -d synapse element
  wait_for "$HS_URL/health" 120 "o Synapse"
}

port_8080_is_ours_or_free() {
  if docker compose ps --status running --services 2>/dev/null | grep -qx frequencia; then return 0; fi
  if (exec 3<>/dev/tcp/127.0.0.1/8080) 2>/dev/null; then
    die "a porta 8080 está ocupada por outro processo (um 'go run'?). Pare-o e rode de novo."
  fi
}

start_app() { # BOT_TOKEN
  port_8080_is_ours_or_free
  log "Subindo o app em modo real (build da imagem na primeira vez)"
  BOT_USER_ID=$(mxid_of "$BOT") BOT_ACCESS_TOKEN=$1 \
    docker compose -f docker-compose.yml -f docker-compose.real.yml up -d --build frequencia
  wait_for "$APP_LOCAL_URL/healthz" 90 "o app"
}

ensure_room() { # TEACHER_TOKEN
  local room invites
  room=$(find_room "$1")
  if [[ -n $room ]]; then
    log "A sala $ROOM_NAME já existe, reaproveitando" >&2
    printf '%s' "$room"
    return
  fi
  log "Criando a sala $ROOM_NAME" >&2
  invites=$(printf '%s\n' "$(mxid_of "$BOT")" | jq -R . | jq -sc .)
  api_ok POST "$1" /_matrix/client/v3/createRoom \
    "$(jq -nc --arg n "$ROOM_NAME" --argjson i "$invites" '{name:$n,preset:"private_chat",invite:$i}')" |
    jq -r '.room_id'
}

ensure_student_in_room() { # USER TOKEN ROOM_ID TEACHER_TOKEN
  local user=$1 token=$2 room=$3 teacher_token=$4 enc out
  has_joined "$token" "$room" && return
  enc=$(urlencode "$room")
  out=$(api POST "$token" "/_matrix/client/v3/join/$enc")
  if jq -e '.errcode' <<<"$out" >/dev/null 2>&1; then
    api_ok POST "$teacher_token" "/_matrix/client/v3/rooms/$enc/invite" \
      "$(jq -nc --arg u "$(mxid_of "$user")" '{user_id:$u}')" >/dev/null
    api_ok POST "$token" "/_matrix/client/v3/join/$enc" >/dev/null
  fi
}

# invite_bot_if_needed TEACHER_TOKEN BOT_TOKEN ROOM_ID: a room that already
# existed may not have the bot. An invite that is already pending is fine.
invite_bot_if_needed() {
  has_joined "$2" "$3" && return
  api POST "$1" "/_matrix/client/v3/rooms/$(urlencode "$3")/invite" \
    "$(jq -nc --arg u "$(mxid_of "$BOT")" '{user_id:$u}')" >/dev/null || true
}

wait_for_bot() { # BOT_TOKEN ROOM_ID
  local i
  for ((i = 0; i < 30; i++)); do
    has_joined "$1" "$2" && return 0
    sleep 1
  done
  die "o bot não entrou na sala em 30 s. Veja: docker compose logs frequencia"
}

add_widget() { # TEACHER_TOKEN ROOM_ID WIDGET_URL
  api_ok PUT "$1" "/_matrix/client/v3/rooms/$(urlencode "$2")/state/im.vector.modular.widgets/$WIDGET_ID" \
    "$(jq -nc --arg url "$3" --arg by "$(mxid_of "$TEACHER")" \
      '{type:"m.custom",url:$url,name:"Frequência",data:{},creatorUserId:$by}')" >/dev/null
}

pin_works() { # ROOM_ID PIN
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'HX-Request: true' \
    --data-urlencode "pin=$2" "$APP_LOCAL_URL/widget/pin/login?roomId=$(urlencode "$1")")
  [[ $code == 200 ]]
}

generate_pin() { # TEACHER_TOKEN ROOM_ID
  local oid
  oid=$(api_ok POST "$1" "/_matrix/client/v3/user/$(urlencode "$(mxid_of "$TEACHER")")/openid/request_token" '{}' |
    jq -r '.access_token')
  curl -fsS -X POST -H "X-OpenID-Token: $oid" -H 'HX-Request: true' \
    "$APP_LOCAL_URL/widget/pin/regenerate?roomId=$(urlencode "$2")" |
    grep -o 'id="pin-value"[^>]*>[0-9]*' | grep -o '[0-9]*$'
}

ensure_pin() { # TEACHER_TOKEN ROOM_ID
  local pin
  pin=$(state_get PIN)
  if [[ $NEW_PIN == 0 && -n $pin ]] && pin_works "$2" "$pin"; then
    printf '%s' "$pin"
    return
  fi
  pin=$(generate_pin "$1" "$2")
  [[ -n $pin ]] || die "não consegui gerar o PIN da sala"
  state_set PIN "$pin"
  printf '%s' "$pin"
}

main() {
  if [[ $CLEAN == 1 ]]; then clean_everything; fi
  start_synapse_and_element

  log "Preparando as contas"
  local bot_token teacher_token
  bot_token=$(ensure_account "$BOT")
  teacher_token=$(ensure_account "$TEACHER")
  set_display_name "$BOT" "$bot_token" "Frequência (bot)"
  set_display_name "$TEACHER" "$teacher_token" "Professor"
  declare -A student_token
  local s n=1
  for s in "${STUDENTS[@]}"; do
    student_token[$s]=$(ensure_account "$s")
    set_display_name "$s" "${student_token[$s]}" "Aluno $n"
    n=$((n + 1))
  done

  local room
  room=$(ensure_room "$teacher_token")
  for s in "${STUDENTS[@]}"; do
    ensure_student_in_room "$s" "${student_token[$s]}" "$room" "$teacher_token"
  done

  invite_bot_if_needed "$teacher_token" "$bot_token" "$room"
  start_app "$bot_token"
  log "Esperando o bot aceitar o convite sozinho"
  wait_for_bot "$bot_token" "$room"

  local public_app="$APP_LOCAL_URL" element_url=http://localhost:8081 homeserver_url=$HS_URL TS_HOST=""
  if [[ $USE_TAILSCALE == 1 ]]; then
    log "Configurando o HTTPS pelo Tailscale"
    if setup_tailscale; then
      public_app=https://$TS_HOST
      homeserver_url=https://$TS_HOST:8443
      element_url=https://$TS_HOST:10000
    else
      warn "seguindo sem Tailscale: o widget usará $public_app, que o celular não alcança"
    fi
  fi

  log "Adicionando o widget à sala $ROOM_NAME"
  add_widget "$teacher_token" "$room" "$public_app/?roomId=\$matrix_room_id&widgetId=\$matrix_widget_id"

  local pin
  pin=$(ensure_pin "$teacher_token" "$room")

  summary "$room" "$public_app" "$element_url" "$homeserver_url" "$pin"
  if [[ $CLEAN == 1 ]]; then clean_browser_note; fi
}

# The browsers and phones keep their own sessions, which this script cannot erase.
clean_browser_note() {
  cat <<'NOTE'
  Para um teste limpo de verdade, apague também o que os clientes guardaram:
    - Navegador: limpe os dados do site do Element (cadeado > Configurações do site > Limpar dados)
      e do widget, ou use uma janela anônima.
    - App Element no celular: saia da conta e remova-a antes de entrar de novo.
    - Contas e salas antigas deixam de existir, então sessões antigas não funcionam mais.

NOTE
}

students_table() {
  local s n=1
  for s in "${STUDENTS[@]}"; do
    printf '  Aluno %s      %s   %s\n' "$n" "$s" "$(password_of "$s")"
    n=$((n + 1))
  done
}

summary() { # ROOM APP ELEMENT HOMESERVER PIN
  cat <<OUT

Tudo pronto.

  Element Web   $3
  Homeserver    $4   (use este endereço no app do celular)
  Widget        $2/?roomId=\$matrix_room_id&widgetId=\$matrix_widget_id
  Sala          $ROOM_NAME ($1)

  Conta        Usuário        Senha
  Professor    $TEACHER   $(password_of "$TEACHER")
$(students_table)
  Bot          $BOT   $(password_of "$BOT")

  PIN da sala (Element Android, que não confirma a identidade): $5
  O widget fica em: sala SD > Informações da sala > Extensões.

OUT
}

main
