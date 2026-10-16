// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import "github.com/therealmcsparrow/mcharbor/core/secrets"

// vaultRef is one place a seeded secret is used: every demo stack built from
// template receives the secret as field.
type vaultRef struct {
	template string
	field    string
}

// vaultSecret is one seeded vault entry. Ages are days before the seed runs; a
// negative age means the event never happened, which the page shows as "never".
type vaultSecret struct {
	name        string
	description string
	kind        string
	group       string
	scope       string
	// template picks the stack a stack-scoped secret belongs to, and envIndex
	// the environment an environment-scoped one does.
	template    string
	envIndex    int
	versions    int
	generated   bool
	autoApply   bool
	prefix      string
	owner       string
	refs        []vaultRef
	stale       int
	usedDays    int
	revealDays  int
	createdDays int
}

// demoVaultSecrets is written against the stacks the demo estate actually runs,
// so a reference opens a real stack. It deliberately carries the states an
// operator should notice: stale references after a rotation, an unused legacy
// secret, and credentials nobody has revealed.
func demoVaultSecrets() []vaultSecret {
	return []vaultSecret{
		{name: "REGISTRY_TOKEN", description: "Pull token for ghcr.io/acme, used by every application stack.",
			kind: "token", group: "Registry", scope: secrets.ScopeGlobal, versions: 4, autoApply: true, prefix: "ghp_", owner: "a.jansen",
			refs:  []vaultRef{{"shop", "REGISTRY_TOKEN"}, {"payments", "REGISTRY_TOKEN"}, {"messaging", "REGISTRY_TOKEN"}},
			stale: 1, usedDays: 0, revealDays: 41, createdDays: 210},
		{name: "SMTP_PASSWORD", description: "Relay credentials for transactional mail from the shop and Keycloak.",
			kind: "password", group: "Mail", scope: secrets.ScopeGlobal, versions: 2, owner: "m.okafor",
			refs:     []vaultRef{{"shop", "SMTP_PASSWORD"}, {"auth", "KC_SMTP_PASSWORD"}},
			usedDays: 1, revealDays: 12, createdDays: 180},
		{name: "SLACK_WEBHOOK_URL", description: "Incoming webhook for #ops-alerts.",
			kind: "env", group: "Alerting", scope: secrets.ScopeGlobal, versions: 1, prefix: "https://hooks.slack.com/services/", owner: "l.moreau",
			refs:     []vaultRef{{"observability", "ALERTMANAGER_SLACK_URL"}},
			usedDays: 0, revealDays: -1, createdDays: 150},
		{name: "OIDC_CLIENT_SECRET", description: "Client secret oauth2-proxy presents to Keycloak.",
			kind: "token", group: "Identity", scope: secrets.ScopeGlobal, versions: 2, generated: true, owner: "a.jansen",
			refs:     []vaultRef{{"auth", "OAUTH2_PROXY_CLIENT_SECRET"}},
			usedDays: 2, revealDays: -1, createdDays: 160},
		{name: "WILDCARD_TLS_KEY", description: "Private key for *.acme.example, renewed every 90 days.",
			kind: "private_key", group: "TLS", scope: secrets.ScopeGlobal, versions: 3, autoApply: true, owner: "k.nakamura",
			refs:  []vaultRef{{"auth", "TLS_KEY"}, {"observability", "GRAFANA_TLS_KEY"}},
			stale: 1, usedDays: 3, revealDays: 30, createdDays: 270},
		{name: "WILDCARD_TLS_CERT", description: "Certificate chain for *.acme.example.",
			kind: "cert", group: "TLS", scope: secrets.ScopeGlobal, versions: 3, autoApply: true, owner: "k.nakamura",
			refs:     []vaultRef{{"auth", "TLS_CERT"}, {"observability", "GRAFANA_TLS_CERT"}},
			usedDays: 3, revealDays: 30, createdDays: 270},
		{name: "DEPLOY_SSH_KEY", description: "Deploy key the CI runner uses to reach the Git mirror.",
			kind: "ssh_key", group: "Access", scope: secrets.ScopeGlobal, versions: 1, generated: true, owner: "a.jansen",
			usedDays: 6, revealDays: -1, createdDays: 95},
		{name: "LOKI_GCS_CREDENTIALS", description: "Service account Loki writes chunks to Cloud Storage with.",
			kind: "json", group: "Cloud", scope: secrets.ScopeGlobal, versions: 2, owner: "l.moreau",
			refs:     []vaultRef{{"observability", "GOOGLE_APPLICATION_CREDENTIALS_JSON"}},
			usedDays: 0, revealDays: 64, createdDays: 120},
		{name: "LEGACY_FTP_PASSWORD", description: "Old FTP backup target. Nothing has used it since the move to S3.",
			kind: "password", group: "Backups", scope: secrets.ScopeGlobal, versions: 1, owner: "t.becker",
			usedDays: -1, revealDays: 200, createdDays: 400},
		{name: "PAGERDUTY_ROUTING_KEY", description: "Events API routing key for the production service.",
			kind: "token", group: "Alerting", scope: secrets.ScopeEnvironment, envIndex: 0, versions: 1, owner: "l.moreau",
			refs:     []vaultRef{{"observability", "PAGERDUTY_ROUTING_KEY"}},
			usedDays: 0, revealDays: -1, createdDays: 140},
		{name: "BACKUP_S3_SECRET_KEY", description: "Secret access key for this host's backup bucket.",
			kind: "token", group: "Backups", scope: secrets.ScopeEnvironment, envIndex: 1, versions: 3, owner: "t.becker",
			usedDays: 0, revealDays: 22, createdDays: 190},
		{name: "POSTGRES_PASSWORD", description: "Owner password for the shop database.",
			kind: "password", group: "Databases", scope: secrets.ScopeStack, template: "shop", versions: 2, generated: true, owner: "j.devries",
			refs:     []vaultRef{{"shop", "POSTGRES_PASSWORD"}},
			usedDays: 0, revealDays: 8, createdDays: 200},
		{name: "STRIPE_API_KEY", description: "Live secret key for the payments gateway.",
			kind: "token", group: "Payments", scope: secrets.ScopeStack, template: "payments", versions: 5, autoApply: true, prefix: "sk_live_", owner: "d.kowalski",
			refs:  []vaultRef{{"payments", "STRIPE_API_KEY"}},
			stale: 1, usedDays: 0, revealDays: 3, createdDays: 330},
		{name: "LEDGER_DB_PASSWORD", description: "Owner password for the ledger database.",
			kind: "password", group: "Databases", scope: secrets.ScopeStack, template: "payments", versions: 1, generated: true, owner: "d.kowalski",
			refs:     []vaultRef{{"payments", "POSTGRES_PASSWORD"}},
			usedDays: 0, revealDays: -1, createdDays: 330},
		{name: "GRAFANA_ADMIN_PASSWORD", description: "Break-glass admin login for Grafana.",
			kind: "password", group: "Monitoring", scope: secrets.ScopeStack, template: "observability", versions: 2, generated: true, owner: "l.moreau",
			refs:     []vaultRef{{"observability", "GF_SECURITY_ADMIN_PASSWORD"}},
			usedDays: 4, revealDays: 90, createdDays: 250},
		{name: "RABBITMQ_DEFAULT_PASS", description: "Default user password for the message broker.",
			kind: "password", group: "Messaging", scope: secrets.ScopeStack, template: "messaging", versions: 1, generated: true, owner: "m.okafor",
			refs:     []vaultRef{{"messaging", "RABBITMQ_DEFAULT_PASS"}},
			usedDays: 1, revealDays: -1, createdDays: 110},
	}
}
