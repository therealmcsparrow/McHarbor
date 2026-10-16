-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE INDEX IF NOT EXISTS idx_ai_provider_connections_api_key_secret_id ON ai_provider_connections(api_key_secret_id);

CREATE INDEX IF NOT EXISTS idx_ai_provider_connections_oauth_client_secret_secret_id ON ai_provider_connections(oauth_client_secret_secret_id);

CREATE INDEX IF NOT EXISTS idx_communication_channels_config_secret_id ON communication_channels(config_secret_id);

CREATE INDEX IF NOT EXISTS idx_environments_ssh_key_secret_id ON environments(ssh_key_secret_id);

CREATE INDEX IF NOT EXISTS idx_environments_tls_ca_secret_id ON environments(tls_ca_secret_id);

CREATE INDEX IF NOT EXISTS idx_environments_tls_cert_secret_id ON environments(tls_cert_secret_id);

CREATE INDEX IF NOT EXISTS idx_environments_tls_key_secret_id ON environments(tls_key_secret_id);

CREATE INDEX IF NOT EXISTS idx_gitops_pr_previews_preview_environment_id ON gitops_pr_previews(preview_environment_id);

CREATE INDEX IF NOT EXISTS idx_identity_providers_client_secret_secret_id ON identity_providers(client_secret_secret_id);

CREATE INDEX IF NOT EXISTS idx_settings_value_secret_id ON settings(value_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_access_key_id_secret_id ON storage_locations(access_key_id_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_ca_certificate_secret_id ON storage_locations(ca_certificate_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_client_certificate_secret_id ON storage_locations(client_certificate_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_client_key_secret_id ON storage_locations(client_key_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_client_secret_secret_id ON storage_locations(client_secret_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_passphrase_secret_id ON storage_locations(passphrase_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_password_secret_id ON storage_locations(password_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_private_key_secret_id ON storage_locations(private_key_secret_id);

CREATE INDEX IF NOT EXISTS idx_storage_locations_secret_access_key_secret_id ON storage_locations(secret_access_key_secret_id);

CREATE INDEX IF NOT EXISTS idx_workflows_variables_secret_id ON workflows(variables_secret_id);
