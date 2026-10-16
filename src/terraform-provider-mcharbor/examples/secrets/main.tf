# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

terraform {
  required_version = ">= 1.11"
  required_providers {
    mcharbor = {
      source = "therealmcsparrow/mcharbor"
    }
  }
}

variable "endpoint" {
  type = string
}

variable "insecure_skip_verify" {
  type    = bool
  default = false
}

variable "database_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

variable "database_password_version" {
  type    = number
  default = 1
}

provider "mcharbor" {
  endpoint             = var.endpoint
  insecure_skip_verify = var.insecure_skip_verify
}

resource "mcharbor_secret" "database_password" {
  name             = "DATABASE_PASSWORD"
  description      = "Option A: the value comes from Terraform, write-only"
  kind             = "password"
  value_wo         = var.database_password
  value_wo_version = var.database_password_version
}

resource "mcharbor_secret" "session_key" {
  name            = "SESSION_KEY"
  description     = "Option B: McHarbor generates the value; it never passes through Terraform"
  kind            = "token"
  generate        = true
  generate_length = 48
}

data "mcharbor_secret" "session_key" {
  name  = mcharbor_secret.session_key.name
  scope = "global"
}

output "database_password_version" {
  value = mcharbor_secret.database_password.version
}

output "session_key_reference" {
  value = "$${secret:${data.mcharbor_secret.session_key.name}}"
}
