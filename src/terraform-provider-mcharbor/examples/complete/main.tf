# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

terraform {
  required_providers {
    mcharbor = {
      source = "therealmcsparrow/mcharbor"
    }
  }
}

provider "mcharbor" {
  endpoint             = var.endpoint
  insecure_skip_verify = var.insecure_skip_verify
}

variable "endpoint" {
  type = string
}

variable "insecure_skip_verify" {
  type    = bool
  default = false
}

variable "operator_user_id" {
  type        = string
  description = "The McHarbor user who operates the new environment."
}

resource "mcharbor_environment" "edge" {
  name            = "edge-01"
  connection_type = "agent"
}

resource "mcharbor_role" "edge_operator" {
  name        = "Edge operator"
  description = "Runs and inspects stacks on the edge fleet."
  permissions = [
    "containers.view",
    "containers.manage",
    "stacks.view",
    "stacks.manage",
    "logs.view",
  ]
}

resource "mcharbor_role_assignment" "operator_on_edge" {
  user_id        = var.operator_user_id
  role_id        = mcharbor_role.edge_operator.id
  environment_id = mcharbor_environment.edge.id
}

resource "mcharbor_stack" "whoami" {
  name           = "whoami"
  environment_id = mcharbor_environment.edge.id
  compose        = <<-YAML
    services:
      whoami:
        image: traefik/whoami:v1.10
        ports:
          - "8080:80"
        environment:
          WHOAMI_NAME: $${WHOAMI_NAME}
  YAML
  env_vars = {
    WHOAMI_NAME = "edge-01"
  }

  auto_start = false
}

output "agent_install_token" {
  value       = mcharbor_environment.edge.agent_token
  sensitive   = true
  description = "Install the agent on the edge host with this token; McHarbor shows it only once."
}

data "mcharbor_plugin_catalog" "all" {}

resource "mcharbor_plugin" "ntfy" {
  slug    = "comm-ntfy"
  version = one([for p in data.mcharbor_plugin_catalog.all.plugins : p.latest_version if p.slug == "comm-ntfy"])
  enabled = true
}

resource "mcharbor_workflow" "nightly" {
  name        = "nightly-check"
  description = "Exported from the workflow editor; edit it there and re-export."
  status      = "draft"
  canvas_json = file("${path.module}/nightly-workflow.json")
}

data "mcharbor_containers" "local" {}

output "local_containers" {
  value = [for c in data.mcharbor_containers.local.containers : "${c.name} (${c.state})"]
}
