// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

// Scope values. A secret resolves stack -> environment -> global, first match
// wins, so the narrower the scope the higher its precedence.
const (
	ScopeGlobal      = "global"
	ScopeEnvironment = "environment"
	ScopeStack       = "stack"
	// ScopeApplication is an instance-wide credential belonging to McHarbor
	// itself. It resolves after ScopeGlobal, so adding one can never change
	// what an existing reference already resolves to.
	ScopeApplication = "application"
)

// Kind values. Presentation and validation only — every kind is stored the
// same way. `cert`, `private_key` and `kubeconfig` are the multi-KB ones, and
// the reason `secrets.value` must never become a keyed column.
//
// These eight are the defaults offered in the UI, not the permitted set:
// migration 130 dropped the CHECK, so any kind matching kindRe is accepted.
const (
	KindEnv        = "env"
	KindPassword   = "password"
	KindToken      = "token"
	KindCert       = "cert"
	KindPrivateKey = "private_key"
	KindKubeconfig = "kubeconfig"
	KindSSHKey     = "ssh_key"
	KindJSON       = "json"
)

// Secret is the metadata view of a vault entry.
//
// There is deliberately **no value field on this type**. Not `omitempty`, not
// unexported — absent. A list or detail response cannot leak a credential
// through a forgotten field if the field does not exist, and every future
// change to this struct inherits that property for free. The plaintext exists
// only as a local in the resolver and in RevealResponse.
type Secret struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"`
	// Group is an optional free-text folder label. It organises the vault
	// listing and nothing else — it is not part of a secret's identity and
	// never takes part in resolution.
	Group          *string `json:"group,omitempty"`
	Scope          string  `json:"scope"`
	EnvironmentID  *string `json:"environmentId,omitempty"`
	StackName      *string `json:"stackName,omitempty"`
	Version        int     `json:"version"`
	Generated      bool    `json:"generated"`
	System         bool    `json:"system"`
	OriginModule   *string `json:"originModule,omitempty"`
	LastUsedAt     *string `json:"lastUsedAt,omitempty"`
	LastRevealedAt *string `json:"lastRevealedAt,omitempty"`
	CreatedBy      *string `json:"createdBy,omitempty"`
	// CreatedByName is populated only by List, from a LEFT JOIN on
	// CreatedBy, so the table can show who created a secret without a
	// second lookup per row.
	CreatedByName *string `json:"createdByName,omitempty"`
	// EnvironmentName mirrors EnvironmentID with a name, populated only by
	// List for the same reason.
	EnvironmentName *string `json:"environmentName,omitempty"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	ReferenceCount  int     `json:"referenceCount"`
	// AutoApply propagates a rotation to everything referencing this secret
	// without waiting to be asked. Off by default, and the default is the
	// safe one: applying restarts containers and redeploys stacks.
	AutoApply bool `json:"autoApply"`
	// StaleCount is how many referencing resources still run an older
	// version. Populated by List and ByID so the table can say so without a
	// second round trip per row.
	StaleCount int `json:"staleCount"`
}

// SecretVersion is one entry of a secret's rotation history. Same rule: the
// stored ciphertext is not on the wire type. Rollback restores a version as
// the current value; it never displays it.
type SecretVersion struct {
	ID        string  `json:"id"`
	SecretID  string  `json:"secretId"`
	Version   int     `json:"version"`
	RotatedBy *string `json:"rotatedBy,omitempty"`
	CreatedAt string  `json:"createdAt"`
}

// SecretReference records that a resource uses a secret, so a delete can warn
// and a rotation can say which stacks need a redeploy.
type SecretReference struct {
	ID            string  `json:"id"`
	SecretID      string  `json:"secretId"`
	ResourceType  string  `json:"resourceType"`
	ResourceID    string  `json:"resourceId"`
	ResourceName  *string `json:"resourceName,omitempty"`
	EnvironmentID *string `json:"environmentId,omitempty"`
	Field         *string `json:"field,omitempty"`
	// AppliedVersion is the secret version this resource was last built with.
	// Zero means the reference predates the stamp, which is reported as stale
	// rather than current — a resource deployed before the column existed may
	// genuinely be running an older value.
	AppliedVersion int    `json:"appliedVersion"`
	CreatedAt      string `json:"createdAt"`
}

// CreateRequest is the body of POST /api/secrets.
type CreateRequest struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Kind          string  `json:"kind"`
	Group         string  `json:"group"`
	Scope         string  `json:"scope"`
	EnvironmentID *string `json:"environmentId"`
	StackName     *string `json:"stackName"`
	Value         string  `json:"value"`
	// Generate asks the server to mint a value with crypto/rand instead of
	// taking one from the client. The generated value comes back once, in the
	// creation response, and never again through an ordinary read.
	Generate bool `json:"generate"`
	// GenerateLength is the requested byte length for a generated value.
	// Clamped to [16, 128]; zero means the default.
	GenerateLength int `json:"generateLength"`
	// AutoApply opts this secret into propagating its own rotations.
	AutoApply bool `json:"autoApply"`
}

// RotateRequest is the body of PUT /api/secrets/{id} — a new value for an
// existing secret. The previous value moves into secret_versions.
type RotateRequest struct {
	Value          string `json:"value"`
	Generate       bool   `json:"generate"`
	GenerateLength int    `json:"generateLength"`
}

// PatchRequest is the body of PATCH /api/secrets/{id} — metadata only. A nil
// field is "leave alone". Changing scope to or from `global` additionally
// requires secrets.manage_global.
type PatchRequest struct {
	Description   *string `json:"description"`
	Kind          *string `json:"kind"`
	Group         *string `json:"group"`
	Scope         *string `json:"scope"`
	EnvironmentID *string `json:"environmentId"`
	StackName     *string `json:"stackName"`
	AutoApply     *bool   `json:"autoApply"`
}

// RollbackRequest is the body of POST /api/secrets/{id}/rollback.
type RollbackRequest struct {
	Version int `json:"version"`
}

// GenerateRequest is the body of POST /api/secrets/generate — a value with no
// secret attached, for a client that wants to show the user a strong password
// before deciding to store it.
type GenerateRequest struct {
	Length int `json:"length"`
}

// RevealRequest is the body of POST /api/secrets/{id}/reveal. The reason is
// mandatory and recorded verbatim in the audit entry: an attributable stated
// purpose changes behaviour more reliably than most technical gates, and it
// is the cheapest control in the design.
type RevealRequest struct {
	Reason string `json:"reason"`
}

// ValueResponse carries a plaintext value. It is returned by exactly three
// endpoints — create-with-generate, rotate-with-generate, and generate — plus
// reveal, and by nothing else. Any handler writing this type should be
// obvious in a diff.
type ValueResponse struct {
	Secret *Secret `json:"secret,omitempty"`
	Value  string  `json:"value"`
}
