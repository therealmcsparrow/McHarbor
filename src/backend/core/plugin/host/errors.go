// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package host

// Errors returned by AccessRegistry methods (AddPermission,
// RegisterSCIMSchema) when an install-time conflict surfaces. These
// live in `host` (rather than `contract`) because the host's
// rbac/scim modules use them to render a translated install error,
// and plugins receive them as wrapped errors from the
// contract.AccessRegistry — they don't reference the variables
// directly.
//
//	ErrPermissionExists — another plugin (or the host) already owns
//	  a permission row with this key.
//	ErrPermissionInUse — uninstall attempt: the key is referenced by
//	  a role that's still bound to users.
//	ErrSchemaExists — another plugin (or the host) already owns a
//	  SCIM schema with this URI.
//	ErrSchemaInUse — uninstall attempt: the URI is referenced by an
//	  external IdP row.
var (
	ErrPermissionExists = constErr("permission key already registered")
	ErrPermissionInUse  = constErr("permission key still referenced by a role")
	ErrSchemaExists     = constErr("scim schema URI already registered")
	ErrSchemaInUse      = constErr("scim schema URI still referenced by an external IdP")
)

type constErr string

func (e constErr) Error() string { return string(e) }
