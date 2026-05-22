-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Kubernetes Support

ALTER TABLE environments ADD COLUMN orchestrator_type TEXT NOT NULL DEFAULT 'docker'
    CHECK(orchestrator_type IN ('docker','kubernetes'));
ALTER TABLE environments ADD COLUMN kubeconfig TEXT;
ALTER TABLE environments ADD COLUMN k8s_namespace TEXT DEFAULT 'default';
ALTER TABLE environments ADD COLUMN k8s_server_url TEXT;
ALTER TABLE environments ADD COLUMN k8s_bearer_token TEXT;
ALTER TABLE environments ADD COLUMN k8s_ca_cert TEXT;
ALTER TABLE environments ADD COLUMN k8s_version TEXT;

CREATE INDEX IF NOT EXISTS idx_environments_orchestrator ON environments(orchestrator_type);
