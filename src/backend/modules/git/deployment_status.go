// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package git

// DeploymentSuccess is the git_deployments status of a completed sync; the
// table's CHECK constraint accepts only these spellings.
const DeploymentSuccess = "success"

// DeploymentFailure is the status of a failed sync. It was written as "failed",
// which the constraint rejects, so failed syncs never reached the history.
const DeploymentFailure = "failure"
