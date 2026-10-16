# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

.PHONY: check-protocol agent agent-cross audit-snapshot audit-reset audit-verify audit-verify-export audit-coverage

# Verify server and agent protocol files are in sync.
# Strips package declarations, all comments (full-line and inline), blank lines,
# and normalizes whitespace so cosmetic differences don't cause false positives.
PROTO_NORMALIZE = grep -v '^package ' | sed 's|[[:space:]]*//.*$$||' | sed '/^[[:space:]]*$$/d' | sed 's/\t/ /g' | tr -s ' '

check-protocol:
	@echo "Checking protocol sync..."
	@diff <(cat src/backend/core/agent/protocol.go | $(PROTO_NORMALIZE)) \
	      <(cat src/agent/protocol.go | $(PROTO_NORMALIZE)) \
	  && echo "Protocol files in sync ✓" \
	  || (echo "ERROR: Protocol files diverged! Update both files." && exit 1)

# --- Agent binaries ------------------------------------------------------
#
# Every agent build lands in agents/ at the repo root. A rebuilt container does
# not reach a native remote agent: it only self-updates when the VERSION goes
# up, and it pulls from GitHub releases rather than from this McHarbor. So a
# fresh binary has to be copied to the host by hand, and a known path in the
# root is what makes that a one-liner.
#
# The work is in scripts/development/build-agent.sh so there is one
# implementation; this machine has no make, and the script runs either way.

agent:
	@bash scripts/development/build-agent.sh

agent-cross:
	@bash scripts/development/build-agent.sh --cross

# --- Audit log safety net (Phase 0 of .references/roadmap/auditlog-plan.md) ---

AUDIT_CONTAINER ?= mcharbor-mcharbor-1
AUDIT_DB ?= /app/data/mcharbor.db
AUDIT_BASELINE_DIR ?= .references/baselines

audit-snapshot:
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/audit_snapshot.ps1 -ContainerName "$(AUDIT_CONTAINER)" -DatabasePath "$(AUDIT_DB)" -OutputDir "$(AUDIT_BASELINE_DIR)"

audit-reset:
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/audit_reset.ps1 -ContainerName "$(AUDIT_CONTAINER)" -DatabasePath "$(AUDIT_DB)"

audit-verify:
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/audit_verify.ps1 -ContainerName "$(AUDIT_CONTAINER)" -DatabasePath "$(AUDIT_DB)"

audit-verify-export:
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/audit_verify_export.ps1

audit-coverage:
	@powershell -NoProfile -ExecutionPolicy Bypass -File scripts/audit_coverage_check.ps1

# --- RBAC safety net (Phase 0 of .references/roadmap/rbac.md) ---

.PHONY: rbac-baseline rbac-reset rbac-rollback

# rbac-baseline: snapshot the admin's effective permission set into
# .references/baselines/admin-permissions.json. Re-run any time the
# catalog changes; subsequent regression checks diff against this file.
# Requires the container to be up.
ADMIN_PASSWORD ?= admin123
rbac-baseline:
	@mkdir -p .references/baselines
	@cookie=$$(mktemp); \
	echo "  logging in as admin..."; \
	login=$$(curl -sSk -c $$cookie -X POST https://localhost:8705/api/auth/login \
		-H "Content-Type: application/json" \
		-d "{\"username\":\"admin\",\"password\":\"$(ADMIN_PASSWORD)\"}"); \
	perms=$$(curl -sSk -b $$cookie https://localhost:8705/api/auth/me/permissions); \
	rm -f $$cookie; \
	echo "$$perms" | python -c "import json,sys; d=json.load(sys.stdin); json.dump(sorted(d.get('permissions', [])), sys.stdout, indent=2)" \
		> .references/baselines/admin-permissions.json; \
	echo "  wrote $$(wc -l < .references/baselines/admin-permissions.json) lines to .references/baselines/admin-permissions.json"

# rbac-reset: wipe permission-related runtime rows but keep the
# permission catalog, role definitions, and migration state intact.
# Use this when an RBAC change has left the install in a bad state
# and you want a clean baseline without a full factory reset.
# Wipes: user_roles, group_members, group_roles, rbac_policies,
#       rbac_policy_bindings, rbac_policy_history, user_permissions,
#       sessions, api_keys.
# Keeps: users (but session is wiped so they will need to log in
#       again), roles, groups definitions, permission_catalog,
#       rbac_v2_enabled setting.
rbac-reset:
	@echo "  wiping user_roles, group_members, group_roles, rbac_policy_*, user_permissions, sessions, api_keys..."
	@docker exec mcharbor-mcharbor-1 sqlite3 /app/data/mcharbor.db <<'SQL'
DELETE FROM user_roles;
DELETE FROM group_members;
DELETE FROM group_roles;
DELETE FROM user_permissions;
DELETE FROM rbac_policies;
DELETE FROM rbac_policy_bindings;
DELETE FROM rbac_policy_history;
DELETE FROM rbac_policy_seeds;
DELETE FROM rbac_policy_history_seed_log;
DELETE FROM rbac_policy_history_log;
DELETE FROM api_keys;
DELETE FROM sessions;
SQL
	@echo "  re-seeding default admin membership..."
	@docker exec mcharbor-mcharbor-1 sqlite3 /app/data/mcharbor.db <<'SQL'
INSERT INTO group_members (id, group_id, user_id, created_at, updated_at)
SELECT 'gm_first_admin', 'grp_admins', id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM users ORDER BY created_at ASC LIMIT 1;
SQL
	@echo "  done. The first user (admin) is back in grp_admins."

# rbac-rollback: how to recover from an RBAC lockout. The actual
# escape hatch is environment variable AUTH_DISABLE=true on the
# container. Print the steps.
rbac-rollback:
	@echo ""
	@echo "RBAC rollback procedure"
	@echo "======================"
	@echo ""
	@echo "If a permission change locked you out, add AUTH_DISABLE=true"
	@echo "to the mcharbor service environment in docker-compose.yml and"
	@echo "recreate the container. The middleware will inject a synthetic"
	@echo "system user for every request, giving you a free pass to fix"
	@echo "the broken assignment. Then remove the env var and recreate"
	@echo "again to restore real auth."
	@echo ""
	@echo "Quick recipe:"
	@echo "  1. Edit docker-compose.yml: add AUTH_DISABLE: 'true' to the"
	@echo "     mcharbor service environment block."
	@echo "  2. docker compose up -d --force-recreate mcharbor"
	@echo "  3. Sign in as the system user (any name/password works)"
	@echo "  4. Fix the broken permission / role / group."
	@echo "  5. Remove AUTH_DISABLE: 'true' from docker-compose.yml."
	@echo "  6. docker compose up -d --force-recreate mcharbor"

.PHONY: gate lint sanity

GATE_ARGS ?=

gate:
	@pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/sanity/gate.ps1 $(GATE_ARGS)

lint:
	@rc=0; \
	(cd src/backend && GOTOOLCHAIN=auto GOWORK=off golangci-lint run ./...) || rc=1; \
	(cd src/agent && GOTOOLCHAIN=auto GOWORK=off golangci-lint run ./...) || rc=1; \
	(cd src/cli && GOTOOLCHAIN=auto GOWORK=off golangci-lint run ./...) || rc=1; \
	(cd src/frontend && npm run lint) || rc=1; \
	(cd src/frontend && npm run lint:out-of-tree) || rc=1; \
	(cd src/frontend && npm run format:check) || rc=1; \
	(cd src/frontend && npm run format:out-of-tree:check) || rc=1; \
	(cd src/frontend && npx knip --include files) || rc=1; \
	(cd src/frontend && npx knip --include exports --no-exit-code) || rc=1; \
	exit $$rc

sanity:
	@pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/sanity/run-all.ps1
