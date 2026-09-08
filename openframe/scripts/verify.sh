#!/usr/bin/env bash
#
# openframe/scripts/verify.sh — post-merge sanity check for the OpenFrame fork.
#
# Run this AFTER resolving an upstream merge conflict (see
# openframe/docs/upstream-sync-conflict-resolution.md). It catches the breakage
# that a clean `git merge` does not: dropped fork logic, signature drift, and
# regressions in the fork's own logic.
#
# Fast tier (no Docker): build + vet + marker check + the pure-logic key-prefix
# tests. Deep tier (MYSQL_TEST=1 + Docker): the MySQL-backed fork tests.
#
# Usage:
#   bash openframe/scripts/verify.sh            # fast tier
#   MYSQL_TEST=1 bash openframe/scripts/verify.sh   # + MySQL-backed tests (needs Docker)
#
set -uo pipefail
cd "$(dirname "$0")/../.." || exit 2

GO_TAGS="full,fts5,netgo"
fail=0
step() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }
ok()   { printf '  \033[32mPASS\033[0m %s\n' "$1"; }
bad()  { printf '  \033[31mFAIL\033[0m %s\n' "$1"; fail=1; }

# 1. Compile the fork-touched code. Catches the #1 silent break: upstream changed
#    a signature/type the fork calls.
step "build fork packages (catches signature/type/import drift)"
if go build -tags "$GO_TAGS" ./cmd/fleet/... ./orbit/cmd/orbit/... \
      ./server/datastore/... ./server/service/... ./server/fleet/... ./server/config/... 2>build.err; then
  ok "go build"
else
  bad "go build — see build.err"; sed 's/^/    /' build.err | head -40
fi
rm -f build.err

# 2. Vet the fork-touched packages.
step "go vet fork packages"
if go vet ./server/datastore/redis/ ./server/datastore/mysql/ ./server/config/ ./server/fleet/ 2>vet.err; then
  ok "go vet"
else
  bad "go vet — see output"; sed 's/^/    /' vet.err | head -30
fi
rm -f vet.err

# 3. Marker presence: if a merge silently dropped fork code, its OPENFRAME markers
#    vanish too. A slug dropping to zero is a red flag worth a human look.
step "OPENFRAME marker presence (dropped-fork-code detector)"
for slug in host-assignments managed-policies managed-queries redis-key-prefix redis-seed-nodes query-results-ttl osquery-host-id agent-openframe-mode agent-json-content-type agent-skip-setup-experience migration-race; do
  n=$(grep -rIl "OPENFRAME($slug" --include='*.go' --include='*.yaml' --include='*.tpl' . 2>/dev/null | wc -l | tr -d ' ')
  if [ "$n" -gt 0 ]; then ok "$slug — present in $n file(s)"; else bad "$slug — NO markers found (fork code may have been dropped in the merge)"; fi
done

# 3b. Marker COVERAGE: every unambiguous fork token must sit inside an OPENFRAME
#     marker. Catches a fork edit added without a marker (or fork code whose marker
#     was lost in a merge). Tokens are fork-only identifiers; generic terms (a plain
#     "keyPrefix", "HostIdentifier", …) are intentionally excluded — upstream uses them.
step "OPENFRAME marker coverage (every fork-token line is marked)"
if python3 - <<'PYEOF'
import re, subprocess, sys
SKIP=('/migrations/openframe/','/service/openframe/','/migrations/tables/','/migrations/data/','/server/mock/','/node_modules/','/vendor/','/tools/fleet-mcp/')
SKIP_EXACT={'server/fleet/openframe.go','server/datastore/redis/keyprefix.go',
 'server/datastore/mysql/openframe.go','server/service/openframe_middleware.go'}
TOKENS=re.compile('|'.join([
 r'[Oo]pen[Ff]rame',r'FLEET_OPENFRAME_MODE',r'ORBIT_OPENFRAME',r'policy_hosts',r'query_hosts',
 r'HostsIncludeAny',r'\bHostIdent\b',r'loadHostsFor(Policies|Queries)',
 r'(Add|Remove|Replace|List)(Policy|Query)Hosts',r'normalizeKeyPrefix',r'newPrefixedConn',
 r'\bprefixedConn\b',r'unwrapConn',r'keyPrefixOf',r'FLEET_REDIS_KEY_PREFIX',r'redis\.key_prefix',
 r'splitSeedNodes',
 r'QueryResultsTTL',r'QueryResultsCleanupInterval',r'CleanupExpiredQueryResults',
 r'CronQueryResultsTTLCleanup',r'newQueryResultsTTLCleanupSchedule',r'query_results_ttl',
 r'query_results_cleanup_interval',r'json:"osquery_host_id"']))
bad=[]
for f in subprocess.run(['git','ls-files','*.go'],capture_output=True,text=True).stdout.split():
    if f.endswith('_test.go') or f in SKIP_EXACT or any(s in '/'+f for s in SKIP): continue
    try: L=open(f).read().splitlines()
    except Exception: continue
    cov=set(); st=[]
    for i,ln in enumerate(L):
        if '>>> OPENFRAME(' in ln: st.append(i)
        if '<<< OPENFRAME(' in ln and st:
            s=st.pop()
            for j in range(s,i+1): cov.add(j)
        if 'OPENFRAME(' in ln: cov.add(i)
    for i,ln in enumerate(L):
        if 'OPENFRAME(' in ln or i in cov: continue
        if TOKENS.search(ln): bad.append(f"{f}:{i+1}: {ln.strip()[:90]}")
if bad:
    print('\n'.join('    '+b for b in bad)); sys.exit(1)
PYEOF
then ok "all fork-token lines covered by markers"
else bad "unmarked fork-token line(s) above — wrap each in an OPENFRAME marker (see openframe/docs/upstream-sync-conflict-resolution.md)"; fi

# 4. Pure-logic unit tests (no Docker). The key-prefix test is the highest-value
#    guard: a regression here is a cross-tenant data leak.
step "key-prefix unit tests (no Docker)"
if go test ./server/datastore/redis/ -run 'PrefixArgs|NormalizeKeyPrefix|PrefixedConn|UnwrapConn|ByteKeys|SplitSeedNodes' >redis.test 2>&1; then
  ok "redis key-prefix tests"
else
  bad "redis key-prefix tests"; sed 's/^/    /' redis.test | tail -30
fi
rm -f redis.test

# 5. MySQL-backed fork tests (deep tier — needs Docker + MYSQL_TEST=1).
if [ "${MYSQL_TEST:-0}" = "1" ]; then
  step "MySQL-backed fork tests (MYSQL_TEST=1)"
  if go test ./server/datastore/mysql/ -run 'MigrateOpenframeIdempotent' >mysql.test 2>&1; then
    ok "openframe migration pipeline"
  else
    bad "openframe migration pipeline"; sed 's/^/    /' mysql.test | tail -40
  fi
  rm -f mysql.test
else
  printf '\n  (skipping MySQL-backed tests — set MYSQL_TEST=1 with Docker running to include them)\n'
fi

# Summary
echo
if [ "$fail" = "0" ]; then
  printf '\033[32mOpenFrame verify: PASS\033[0m\n'
else
  printf '\033[31mOpenFrame verify: FAIL — review the merge resolution above\033[0m\n'
fi
exit "$fail"
