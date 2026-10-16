// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"log/slog"
)

// LegacyCatalog is the complete list of module-owned credentials the vault
// adopts, in the order Phase 5b of the secrets plan runs them: least to most
// dangerous, so the pattern is proven on a webhook signing secret before it
// touches the token that keeps every remote agent connected.
//
// Adding an entry here is the whole of adopting a new module, provided the
// matching `<thing>_secret_id` column exists (migration 115) and the module's
// read path goes through Service.ReadThrough.
//
// **What is deliberately absent** — both omissions are security decisions,
// not scope cuts (§5.11 of `.references/todo/secrets-manager.md`):
//
//  1. `user_mfa` TOTP seeds and recovery codes. They are not operator
//     credentials; they are the user's second factor. Putting them in a vault
//     that has a reveal endpoint would let an administrator with an approved
//     elevation read a user's TOTP seed and impersonate their MFA
//     indefinitely, without the user ever knowing — a straight downgrade of
//     the authentication system. Confirmed by the project owner on
//     2026-09-13 as settled, and asserted by TestMFAStaysOutOfTheVault
//     rather than merely commented, because a comment does not survive
//     contact with a tidy-up refactor.
//  2. Machine-rotated OAuth tokens — `storage_locations.access_token` /
//     `.refresh_token` / `.token`, the `identity_providers` rotating tokens,
//     and `ai_provider_connections.oauth_access_token` / `_refresh_token`.
//     They refresh hourly. Versioning them would write `secret_versions` rows
//     continuously for values no human ever needs to see or roll back, and
//     the reference tracking would be noise.
func LegacyCatalog() []LegacyColumn {
	return []LegacyColumn{
		// 5b.1 — webhooks. Lowest blast radius, so it establishes the pattern.
		{
			Table: "webhooks", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "secret", SecretIDColumn: "secret_secret_id",
			NamePrefix: "WEBHOOK", Module: "webhooks", Kind: KindToken,
		},

		// 5b.2 — registry passwords.
		{
			Table: "registries", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "password", SecretIDColumn: "password_secret_id",
			NamePrefix: "REGISTRY", Module: "registry", Kind: KindPassword,
		},

		// 5b.3 — git credentials.
		{ //nolint:gosec // column name, not a credential
			Table: "git_repos", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "credential_ref", SecretIDColumn: "credential_secret_id",
			NamePrefix: "GIT", Module: "git", Kind: KindToken,
		},

		// 5b.4 — workflow node credential values. The whole `variables` blob is
		// one encrypted JSON document, so it adopts as one `json` secret.
		{
			Table: "workflows", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "variables", SecretIDColumn: "variables_secret_id",
			NamePrefix: "WORKFLOW", Module: "workflows", Kind: KindJSON,
		},

		// 5b.5 — storage locations. The nine fields an operator types; the
		// OAuth token columns beside them are excluded, see the doc comment.
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "password", SecretIDColumn: "password_secret_id",
			NamePrefix: "STORAGE_PW", Module: "storage_locations", Kind: KindPassword},
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "private_key", SecretIDColumn: "private_key_secret_id",
			NamePrefix: "STORAGE_KEY", Module: "storage_locations", Kind: KindPrivateKey},
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "passphrase", SecretIDColumn: "passphrase_secret_id",
			NamePrefix: "STORAGE_PASS", Module: "storage_locations", Kind: KindPassword},
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "ca_certificate", SecretIDColumn: "ca_certificate_secret_id",
			NamePrefix: "STORAGE_CA", Module: "storage_locations", Kind: KindCert},
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "client_certificate", SecretIDColumn: "client_certificate_secret_id",
			NamePrefix: "STORAGE_CERT", Module: "storage_locations", Kind: KindCert},
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name", //nolint:gosec // column name, not a credential
			ValueColumn: "client_key", SecretIDColumn: "client_key_secret_id",
			NamePrefix: "STORAGE_CLIENTKEY", Module: "storage_locations", Kind: KindPrivateKey},
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name", //nolint:gosec // column name, not a credential
			ValueColumn: "access_key_id", SecretIDColumn: "access_key_id_secret_id",
			NamePrefix: "STORAGE_AKID", Module: "storage_locations", Kind: KindToken},
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "secret_access_key", SecretIDColumn: "secret_access_key_secret_id",
			NamePrefix: "STORAGE_SAK", Module: "storage_locations", Kind: KindToken},
		{Table: "storage_locations", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "client_secret", SecretIDColumn: "client_secret_secret_id",
			NamePrefix: "STORAGE_CLIENTSECRET", Module: "storage_locations", Kind: KindToken},

		// 5b.6 — communications. SMTP and transport credentials live inside one
		// encrypted JSON config blob.
		{ //nolint:gosec // column name, not a credential
			Table: "communication_channels", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "config_enc", SecretIDColumn: "config_secret_id",
			NamePrefix: "COMM", Module: "communications", Kind: KindJSON,
		},

		// 5b.7 — AI providers. The API key and the OAuth *client* secret move;
		// the rotating access and refresh tokens do not.
		{
			Table: "ai_provider_connections", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "api_key", SecretIDColumn: "api_key_secret_id",
			NamePrefix: "AI_APIKEY", Module: "ai_providers", Kind: KindToken,
		},
		{ //nolint:gosec // column name, not a credential
			Table: "ai_provider_connections", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "oauth_client_secret", SecretIDColumn: "oauth_client_secret_secret_id",
			NamePrefix: "AI_OAUTH", Module: "ai_providers", Kind: KindToken,
		},

		// 5b.8 — the TLS certificate and private key. First multi-KB values in
		// the vault, and the reason `secrets.value` must never become a keyed
		// column: VARCHAR(255) would not hold a certificate chain.
		{
			Table: "settings", IDColumn: "key", LabelColumn: "key",
			ValueColumn: "value", SecretIDColumn: "value_secret_id",
			NamePrefix: "SETTINGS", Module: "settings", Kind: KindCert,
			Where: "key = 'tls_cert'",
		},
		{
			Table: "settings", IDColumn: "key", LabelColumn: "key",
			ValueColumn: "value", SecretIDColumn: "value_secret_id",
			NamePrefix: "SETTINGS", Module: "settings", Kind: KindPrivateKey,
			Where: "key = 'tls_key'",
		},

		// 5b.9 — environment TLS and SSH material. Kubernetes moved to a plugin
		// in migration 077, so the kubeconfig and bearer-token columns §5.11
		// names no longer exist here.
		{Table: "environments", IDColumn: "id", LabelColumn: "name", //nolint:gosec // column name, not a credential
			ValueColumn: "tls_ca", SecretIDColumn: "tls_ca_secret_id",
			NamePrefix: "ENV_CA", Module: "environments", Kind: KindCert},
		{Table: "environments", IDColumn: "id", LabelColumn: "name", //nolint:gosec // column name, not a credential
			ValueColumn: "tls_cert", SecretIDColumn: "tls_cert_secret_id",
			NamePrefix: "ENV_CERT", Module: "environments", Kind: KindCert},
		{Table: "environments", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "tls_key", SecretIDColumn: "tls_key_secret_id",
			NamePrefix: "ENV_TLSKEY", Module: "environments", Kind: KindPrivateKey},
		{Table: "environments", IDColumn: "id", LabelColumn: "name", //nolint:gosec // column name, not a credential
			ValueColumn: "ssh_key", SecretIDColumn: "ssh_key_secret_id",
			NamePrefix: "ENV_SSHKEY", Module: "environments", Kind: KindSSHKey},

		// 5b.10 — the OIDC/SAML client secret. The rotating tokens beside it
		// stay put.
		{
			Table: "identity_providers", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "client_secret", SecretIDColumn: "client_secret_secret_id",
			NamePrefix: "IDP", Module: "identity", Kind: KindToken,
		},

		// 5b.11 — the agent enrolment token. Last, because a mistake here
		// disconnects every remote environment. The show-once-then-redacted
		// contract is unchanged: the plaintext is still only surfaced by the
		// mint-new path in modules/agent, and `agent_token_hash` — which is
		// what actually authenticates a connecting agent — is untouched.
		{ //nolint:gosec // column name, not a credential
			Table: "environments", IDColumn: "id", LabelColumn: "name",
			ValueColumn: "agent_token", SecretIDColumn: "agent_token_secret_id",
			NamePrefix: "AGENT", Module: "agent", Kind: KindToken,
			Where: "connection_type = 'agent'",
		},
	}
}

// AdoptAll runs the whole catalogue, then sweeps up the orphans left behind
// by any module that rotated its credential since the last boot. Called once
// at start-up.
//
// Prune runs **after** adopt, not before: a value rotated while the process
// was down is adopted again on this pass, and only then is the copy it
// replaced unreferenced.
func (s *Service) AdoptAll(ctx context.Context) error {
	catalog := LegacyCatalog()
	adoptErr := s.AdoptLegacyColumns(ctx, catalog...)
	if _, err := s.PruneOrphanedSystemSecrets(ctx, catalog); err != nil {
		slog.Warn("pruning orphaned adopted credentials failed", "error", err)
	}
	return adoptErr
}
