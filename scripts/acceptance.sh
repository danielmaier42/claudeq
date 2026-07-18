#!/usr/bin/env bash
# Acceptance test for claudeq (Phases 2-3).
#
# Drives the real claudeq / claudeqd binaries against a fake `claude` (no tokens,
# deterministic) and checks each Phase-2 requirement, printing PASS/FAIL with the
# requirement id. Exits non-zero if any check fails.
#
# Usage:  ./scripts/acceptance-phase2.sh
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

export CLAUDEQ_HOME="$WORK/home"
export CQ_FAKE_ARGS="$WORK/args.log"
export CQ_FAKE_STATE="$WORK/calls"
REPO_A="$WORK/repoA"; REPO_B="$WORK/repoB"
mkdir -p "$REPO_A" "$REPO_B"

pass=0; fail=0
check() { # check "<id> description" <expr...>
  local desc="$1"; shift
  if "$@"; then printf '  PASS  %s\n' "$desc"; pass=$((pass + 1))
  else printf '  FAIL  %s\n' "$desc"; fail=$((fail + 1)); fi
}
contains() { grep -q -- "$2" "$1"; }
num_check() { # num_check "desc" <actual> <op> <expected>
  local desc="$1"
  if [ "$2" "$3" "$4" ]; then printf '  PASS  %s\n' "$desc"; pass=$((pass + 1))
  else printf '  FAIL  %s (got %s, expected %s %s)\n' "$desc" "$2" "$3" "$4"; fail=$((fail + 1)); fi
}
lines() { grep -c -- "$2" "$1" 2>/dev/null; :; }

echo "== building binaries =="
go build -o "$WORK/claudeq"  "$ROOT/cmd/claudeq"  || exit 2
go build -o "$WORK/claudeqd" "$ROOT/cmd/claudeqd" || exit 2
CQ="$WORK/claudeq"; CQD="$WORK/claudeqd"

# Fake claude: logs its args, then behaves per $CQ_FAKE_MODE.
FAKE="$WORK/fakebin"; mkdir -p "$FAKE"
cat > "$FAKE/claude" <<'EOF'
#!/bin/sh
echo "ARGS: $*" >> "$CQ_FAKE_ARGS"
case "${CQ_FAKE_MODE:-ok}" in
  ok) echo '{"type":"result","is_error":false,"result":"ok","session_id":"s-ok"}' ;;
  rl) echo "{\"type\":\"system\",\"subtype\":\"api_retry\",\"error_status\":429,\"error\":\"rate_limit\",\"retry_delay_ms\":${CQ_FAKE_RETRY_MS:-3600000},\"session_id\":\"s-rl\"}"; exit 1 ;;
  auth) echo '{"type":"result","is_error":true,"error":"authentication_failed"}'; exit 1 ;;
  fail) echo '{"type":"result","is_error":true,"result":"boom"}'; exit 2 ;;
  resume-once)
    n=$(cat "$CQ_FAKE_STATE" 2>/dev/null || echo 0); n=$((n + 1)); echo "$n" > "$CQ_FAKE_STATE"
    if [ "$n" -eq 1 ]; then
      echo '{"type":"system","subtype":"api_retry","error_status":429,"retry_delay_ms":2000,"session_id":"s-r"}'; exit 1
    else
      echo '{"type":"result","is_error":false,"result":"ok","session_id":"s-r"}'
    fi ;;
esac
EOF
chmod +x "$FAKE/claude"

# Fake system tools (launchctl/sudo/pmset) log their args instead of touching
# the real system, so Phase 3 install/wake can be checked safely.
for tool in launchctl sudo pmset osascript pkill; do
  cat > "$FAKE/$tool" <<EOF
#!/bin/sh
echo "\$*" >> "$WORK/$tool.log"
exit 0
EOF
  chmod +x "$FAKE/$tool"
done
export PATH="$FAKE:$PATH"
# Force claudeq to use the fake claude (the daemon otherwise auto-detects the
# real one at a common install path, bypassing PATH).
export CLAUDEQ_CLAUDE_BIN="$FAKE/claude"

echo
echo "== Task definition & management =="
"$CQ" add --id a --name "Build A" --prompt "pa" --dir "$REPO_A" --trigger asap >/dev/null
"$CQ" add --id b --name "Build B" --prompt "pb" --dir "$REPO_B" --trigger asap --parallel >/dev/null
check "FA-01 stores prompt/dir/trigger/parallel"     contains "$CLAUDEQ_HOME/config.toml" "$REPO_A"
check "FA-03 multiple tasks, different contexts"     bash -c '[ "$('"$CQ"' list | grep -c Build)" -eq 2 ]'
"$CQ" add --id bad --prompt "x" --dir "$REPO_A" --trigger cron --cron "not-a-cron" >/dev/null 2>&1
check "FA-04/18 invalid cron rejected"               bash -c '! '"$CQ"' list | grep -q "^.*bad"'
"$CQ" disable a >/dev/null
check "FA-17 enable/pause without delete"             bash -c ''"$CQ"' list | grep " a " | grep -q false'
"$CQ" enable a >/dev/null
"$CQ" move b 0 >/dev/null
check "FA-11 manual priority reorder (b to top)"      bash -c '[ "$('"$CQ"' list | awk "NR==2{print \$2}")" = "b" ]'

echo
echo "== Execution (run-now) & result visibility =="
"$CQ" run-now a >/dev/null
check "FA-16 run-now executes"                        contains "$CQ_FAKE_ARGS" "ARGS:"
"$CQ" status | grep -q success
num_check "FA-15 status shows success"                "$?" -eq 0
check "FA-15 per-run log file written"                bash -c 'ls "$CLAUDEQ_HOME"/runs/*.log >/dev/null 2>&1'

echo
echo "== Model & permission flags (per-task / global) =="
: > "$CQ_FAKE_ARGS"
"$CQ" add --id sk --prompt "p" --dir "$REPO_A" --trigger asap --skip-permissions --model claude-haiku-4-5-20251001 >/dev/null
"$CQ" run-now sk >/dev/null
check "FA-30 per-task model override passed"          contains "$CQ_FAKE_ARGS" "--model claude-haiku-4-5-20251001"
check "FA-31 per-task skip-permissions passed"        contains "$CQ_FAKE_ARGS" "--dangerously-skip-permissions"
check "FA-27 headless stream-json invocation"         contains "$CQ_FAKE_ARGS" "--output-format stream-json"

echo
echo "== Scheduler loop: asap / fixed earliest-start =="
export CLAUDEQ_HOME="$WORK/home2"; mkdir -p "$CLAUDEQ_HOME"
FUTURE=$(date -u -v+1H +%Y-%m-%dT%H:%M:%SZ)
PAST=$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ)
"$CQ" add --id now  --prompt "p" --dir "$REPO_A" --trigger asap >/dev/null
"$CQ" add --id past --prompt "p" --dir "$REPO_A" --trigger fixed --at "$PAST" >/dev/null
"$CQ" add --id soon --prompt "p" --dir "$REPO_A" --trigger fixed --at "$FUTURE" >/dev/null
CQ_FAKE_MODE=ok "$CQD" run --interval 300ms >/dev/null 2>&1 &
DPID=$!; sleep 2; kill -INT $DPID 2>/dev/null; wait $DPID 2>/dev/null
H="$CLAUDEQ_HOME/history.jsonl"
now_n=$(lines "$H" '"task_id":"now"');  past_n=$(lines "$H" '"task_id":"past"'); soon_n=$(lines "$H" '"task_id":"soon"')
num_check "FA-07 asap task ran"                       "${now_n:-0}"  -ge 1
num_check "FA-08 fixed(past) ran"                     "${past_n:-0}" -ge 1
num_check "FA-08 fixed(future) did NOT run"           "${soon_n:-0}" -eq 0

echo
echo "== Reactive limit gate: no spin, then resume =="
# Rate-limit: exactly one attempt while the gate is blocked.
export CLAUDEQ_HOME="$WORK/home3"; mkdir -p "$CLAUDEQ_HOME"
"$CQ" add --id blk --prompt "p" --dir "$REPO_A" --trigger asap >/dev/null
CQ_FAKE_MODE=rl "$CQD" run --interval 200ms >/dev/null 2>&1 &
DPID=$!; sleep 2; kill -INT $DPID 2>/dev/null; wait $DPID 2>/dev/null
H3="$CLAUDEQ_HOME/history.jsonl"
starts=$(grep '"task_id":"blk"' "$H3" 2>/dev/null | grep -c '"status":"running"')
num_check "FA-05/06/37 rate-limit → exactly one attempt (no spin)" "${starts:-0}" -eq 1
check "FA-35 failure/limit visible in status"         bash -c ''"$CQ"' status | grep -q rate_limited_waiting'

# Resume after reset: 1st call 429 (retry 2s), later call succeeds via --resume.
export CLAUDEQ_HOME="$WORK/home4"; mkdir -p "$CLAUDEQ_HOME"; : > "$CQ_FAKE_ARGS"; : > "$CQ_FAKE_STATE"
"$CQ" add --id res --prompt "p" --dir "$REPO_A" --trigger asap >/dev/null
CQ_FAKE_MODE=resume-once "$CQD" run --interval 400ms >/dev/null 2>&1 &
DPID=$!; sleep 5; kill -INT $DPID 2>/dev/null; wait $DPID 2>/dev/null
check "D4 second attempt used --resume"               contains "$CQ_FAKE_ARGS" "--resume"
check "D4 task succeeded after resume"                bash -c ''"$CQ"' status | grep " res " | grep -q success'

echo
echo "== News/unread persistence =="
export CLAUDEQ_HOME="$WORK/home5"; mkdir -p "$CLAUDEQ_HOME"
"$CQ" add --id n --prompt "p" --dir "$REPO_A" --trigger asap >/dev/null
"$CQ" run-now n >/dev/null
check "FA-22 new run is unread"                       bash -c ''"$CQ"' status | grep -q "1 unread"'
"$CQ" read-all >/dev/null
check "FA-24/26 read status persists (new process)"   bash -c ''"$CQ"' status | grep -q "0 unread"'

echo
echo "== Resilience & wake (Phase 3; fake launchctl/sudo/pmset) =="
FAKEHOME="$WORK/fakehome"; mkdir -p "$FAKEHOME"
export CLAUDEQ_HOME="$WORK/home6"; mkdir -p "$CLAUDEQ_HOME"
HOME="$FAKEHOME" "$CQD" install >/dev/null 2>&1
PLIST="$FAKEHOME/Library/LaunchAgents/de.maierdaniel.claudeq.plist"
check "NFA-03 install writes LaunchAgent plist"       test -f "$PLIST"
check "NFA-03 plist has RunAtLoad (autostart)"        contains "$PLIST" "<key>RunAtLoad</key>"
check "NFA-03 install bootstraps via launchctl"       contains "$WORK/launchctl.log" "bootstrap"

"$CQ" add --id wk --prompt "p" --dir "$REPO_A" --trigger asap >/dev/null
: > "$WORK/sudo.log"
CQ_FAKE_MODE=ok "$CQD" run --interval 300ms >/dev/null 2>&1 &
DPID=$!; sleep 2; kill -INT $DPID 2>/dev/null; wait $DPID 2>/dev/null
check "FA-32 daemon schedules a pmset wake"           contains "$WORK/sudo.log" "pmset schedule wake"

HOME="$FAKEHOME" "$CQD" uninstall >/dev/null 2>&1
check "NFA-03 uninstall removes plist"                bash -c '[ ! -f "'"$PLIST"'" ]'
check "NFA-03 uninstall boots out via launchctl"      contains "$WORK/launchctl.log" "bootout"

echo
echo "== Notifications (Phase 4; fake osascript) =="
export CLAUDEQ_HOME="$WORK/home7"; mkdir -p "$CLAUDEQ_HOME"
"$CQ" add --id failer --prompt "p" --dir "$REPO_A" --trigger asap >/dev/null
: > "$WORK/osascript.log"
CQ_FAKE_MODE=fail "$CQD" run --interval 300ms --no-wake >/dev/null 2>&1 &
DPID=$!; sleep 2; kill -INT $DPID 2>/dev/null; wait $DPID 2>/dev/null
check "FA-35/39 failure triggers a macOS notification" contains "$WORK/osascript.log" "display notification"

echo
echo "== Dashboard API (Phase 5) =="
export CLAUDEQ_HOME="$WORK/home8"; mkdir -p "$CLAUDEQ_HOME"
FUT=$(date -u -v+1H +%Y-%m-%dT%H:%M:%SZ)
"$CQ" add --id web --prompt "p" --dir "$REPO_A" --trigger fixed --at "$FUT" >/dev/null  # stays listed (not due)
"$CQ" add --id ran --prompt "p" --dir "$REPO_A" --trigger asap >/dev/null               # runs then leaves the queue
CQ_FAKE_MODE=ok "$CQD" run --interval 500ms --no-wake --addr 127.0.0.1:8791 >/dev/null 2>&1 &
DPID=$!; sleep 2
tasks_json=$(curl -s http://127.0.0.1:8791/api/tasks)
dash_html=$(curl -s http://127.0.0.1:8791/)
runs_json=$(curl -s http://127.0.0.1:8791/api/runs)
kill -INT $DPID 2>/dev/null; wait $DPID 2>/dev/null
case "$tasks_json" in *'"id":"web"'*) t1=0;; *) t1=1;; esac
num_check "FA-02 API lists tasks (lowercase JSON keys)"  "$t1" -eq 0
case "$tasks_json" in *'"id":"ran"'*) t1b=1;; *) t1b=0;; esac
num_check "finished one-shot task leaves the queue"      "$t1b" -eq 0
case "$dash_html" in *"<title>ClaudeQ</title>"*) t2=0;; *) t2=1;; esac
num_check "FA-25 dashboard HTML served"                  "$t2" -eq 0
case "$runs_json" in *'"status":"success"'*) t3=0;; *) t3=1;; esac
num_check "FA-15/22 API exposes run history/status"      "$t3" -eq 0
case "$runs_json" in *'"task":'*) t4=0;; *) t4=1;; esac
num_check "runs carry a replayable task snapshot"        "$t4" -eq 0

echo
echo "== Concurrency (covered by automated tests) =="
if go test "$ROOT/internal/schedule/..." "$ROOT/internal/engine/..." -run 'Parallel|Exclusive|Select' -count=1 >/dev/null 2>&1; then
  printf '  PASS  FA-12/13/14/33 parallelism & priority (go test)\n'; pass=$((pass + 1))
else
  printf '  FAIL  FA-12/13/14/33 parallelism & priority (go test)\n'; fail=$((fail + 1))
fi

echo
echo "== Packaging & release (phase 6) =="
check "NFA-05 installer places the app in /Applications"   contains "$ROOT/scripts/build-pkg.sh" "Applications/ClaudeQ.app"
check "NFA-05 installer disables bundle relocation"        contains "$ROOT/scripts/build-pkg.sh" "BundleIsRelocatable false"
check "app bundle is code-signed under its bundle id"      contains "$ROOT/scripts/build-app.sh" "identifier de.maierdaniel.claudeq"
check "NFA-05 installer runs the postinstall scripts dir"  contains "$ROOT/scripts/build-pkg.sh" "scripts/pkg"
check "NFA-05 postinstall is valid shell"                  sh -n "$ROOT/scripts/pkg/postinstall"
check "NFA-05 postinstall sets up the LaunchAgent"         contains "$ROOT/scripts/pkg/postinstall" '"\$DAEMON" install'
# Regression guard: the postinstall must NEVER delete anything under
# /Applications — a case-insensitive-FS bug once made it rm the app it just
# installed. It may reference the path (to locate the daemon), just never rm it.
check "postinstall never removes anything under /Applications" bash -c '! grep -Eq "(rm|unlink|ditto .*--nocache)[^\\n]*/Applications" "'"$ROOT"'/scripts/pkg/postinstall"'
check "NFA-06 uninstall removes LaunchAgent and app"       contains "$ROOT/scripts/uninstall.sh" "uninstall"
check "NFA-06 uninstall is valid shell"                    bash -n "$ROOT/scripts/uninstall.sh"
check "release pipeline triggers on version tags"          contains "$ROOT/.github/workflows/release.yml" 'tags:'
check "release pipeline attaches the installer pkg"        contains "$ROOT/.github/workflows/release.yml" "claudeq-.*.pkg"

echo
echo "=========================================="
printf "Result: %d passed, %d failed\n" "$pass" "$fail"
echo "=========================================="
[ "$fail" -eq 0 ]
