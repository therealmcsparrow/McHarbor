// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// nameRe is the whole of the name contract: an uppercase identifier, because
// that is what a `${secret:NAME}` reference and a shell environment variable
// both look like.
var nameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// kindRe bounds a custom kind. The built-in eight are lowercase identifiers and
// a custom one has to look like them, because a kind reaches the UI as the
// `kind.<value>` translation key and reaches an export as a column value.
var kindRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// versionRetention is how many historical values a secret keeps. Older ones
// are pruned on rotate.
const versionRetention = 5

// groupMaxLength bounds a group label. It is a folder name in a sidebar, not
// prose, and an unbounded one would break the table layout long before it
// broke the column.
const groupMaxLength = 64

// revealsPerHour is the per-user rate limit on the reveal endpoint. A
// permission holder enumerating the vault in a loop is the exfiltration shape
// that matters, and it looks nothing like legitimate use — nobody needs to
// read eleven credentials in an hour. Tune once there is real usage; the
// number matters less than having one.
const revealsPerHour = 10

// RevealAuditAction is the audit action name the reveal endpoint writes. The
// rate limiter counts rows carrying it, so the handler and the limiter must
// agree — hence the shared constant rather than two string literals.
const RevealAuditAction = "reveal_secret"

// Errors the handler maps onto i18n codes and HTTP statuses. Everything else
// is an internal error: logged server-side, never returned verbatim.
var (
	ErrNotFound        = errors.New("secret not found")
	ErrNameInvalid     = errors.New("secret name invalid")
	ErrDuplicate       = errors.New("secret already exists at this scope")
	ErrInUse           = errors.New("secret is still referenced")
	ErrSystemImmutable = errors.New("secret is managed by a host module")
	ErrGlobalDenied    = errors.New("secrets.manage_global required")
	ErrScopeInvalid    = errors.New("invalid scope")
	ErrKindInvalid     = errors.New("invalid kind")
	ErrVersionNotFound = errors.New("secret version not found")
	ErrRateLimited     = errors.New("reveal rate limit exceeded")
)

// Authorizer is the slice of the RBAC service this module needs. Narrow on
// purpose: the global-scope check has to happen inside the service (route
// middleware cannot see the request body), and taking the whole RBAC service
// to ask one question would make that dependency look bigger than it is.
type Authorizer interface {
	HasPermission(ctx context.Context, userID, envID string, perm rbac.Permission) (bool, error)
}

// Service owns every read and write of the vault tables.
type Service struct {
	db   *sql.DB
	enc  *encryption.Service
	auth Authorizer
}

// NewService builds the secrets service from the specific dependencies it
// needs rather than the whole of AppDeps.
func NewService(db *sql.DB, enc *encryption.Service, authz Authorizer) *Service {
	return &Service{db: db, enc: enc, auth: authz}
}

// ─── Key computation ──────────────────────────────────────────────────────
//
// scopeKey and refKey exist because MariaDB has no functional key parts, so
// "one NAME per (environment, stack)" cannot be a unique index over
// COALESCE(...) and needs a real column. Each is computed in exactly one
// place. A stale scope_key silently permits a duplicate name at the same
// scope, which is the single bug this column exists to prevent, so every
// write that touches a component field must route through here.

// scopeKey digests the tuple that must be unique: name plus the scope it
// lives in. The separator cannot appear in any component (names are
// `[A-Z0-9_]`, ids are xid, stack names are sanitised), so distinct tuples
// cannot collide by concatenation.
func scopeKey(name string, environmentID, stackName *string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		name,
		derefOrEmpty(environmentID),
		derefOrEmpty(stackName),
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// scopeKeyFor disambiguates the one scope the three-field digest cannot tell
// apart. An application secret carries neither an environment nor a stack —
// exactly like a global one — so without the marker the two would collide on
// the unique index. Every other scope digests what it always did, which is why
// this change needs no backfill.
func scopeKeyFor(scope, name string, environmentID, stackName *string) string {
	if scope == ScopeApplication {
		marker := ScopeApplication
		return scopeKey(name, &marker, stackName)
	}
	return scopeKey(name, environmentID, stackName)
}

// ScopeKey exposes the scope digest for callers that must rewrite it outside a
// normal write — an environment merge re-points a secret's environment_id, and
// a stale scope_key would make the moved secret unresolvable and un-deduped.
func ScopeKey(name string, environmentID, stackName *string) string {
	return scopeKey(name, environmentID, stackName)
}

// refKey digests what makes a reference unique, so re-resolving the same
// secret for the same field is an upsert rather than a new row every deploy.
func refKey(secretID, resourceType, resourceID string, field *string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		secretID, resourceType, resourceID, derefOrEmpty(field),
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func emptyToNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	return &trimmed
}

// ─── Validation ───────────────────────────────────────────────────────────

// ValidateName is exported so the handler can reject a bad name before any
// work happens, per the "handler validates, service enforces" split.
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return ErrNameInvalid
	}
	return nil
}

// NormalizeGroup trims a submitted group label and caps it at groupMaxLength.
// An empty result means "ungrouped", which is stored as NULL rather than as an
// empty string so the index has exactly one representation of it.
func NormalizeGroup(group string) string {
	trimmed := strings.TrimSpace(group)
	if len(trimmed) > groupMaxLength {
		trimmed = strings.TrimSpace(trimmed[:groupMaxLength])
	}
	return trimmed
}

func validateScope(scope string) error {
	switch scope {
	case ScopeGlobal, ScopeEnvironment, ScopeStack, ScopeApplication:
		return nil
	}
	return ErrScopeInvalid
}

// NormalizeKind trims and lowercases a submitted kind so `API_KEY`, `Api Key`
// and `api_key` cannot become three kinds that look like one in the list.
func NormalizeKind(kind string) string {
	trimmed := strings.ToLower(strings.TrimSpace(kind))
	return strings.ReplaceAll(trimmed, " ", "_")
}

// validateKind accepts the eight built-ins and any well-formed custom kind.
// The vocabulary is open as of migration 130 — kind is presentation only, and
// the shape check is what stops a kind that cannot be rendered or exported.
func validateKind(kind string) error {
	if !kindRe.MatchString(kind) {
		return ErrKindInvalid
	}
	return nil
}

// requireGlobalAuthority is the half of the global/non-global split that
// middleware cannot do. `RequirePermission(PermSecretsManage)` gates the
// route; the scope arrives in the body, which the middleware never sees. So
// this check lives here — on create, on rotate, and on any patch that moves a
// secret to or from global — or it does not exist at all.
func (s *Service) requireGlobalAuthority(ctx context.Context, userID, envID, scope string) error {
	if scope != ScopeGlobal && scope != ScopeApplication {
		return nil
	}
	if s.auth == nil {
		return ErrGlobalDenied
	}
	allowed, err := s.auth.HasPermission(ctx, userID, envID, rbac.PermSecretsManageGlobal)
	if err != nil {
		return fmt.Errorf("checking secrets.manage_global: %w", err)
	}
	if !allowed {
		return ErrGlobalDenied
	}
	return nil
}

// ─── Generation ───────────────────────────────────────────────────────────

// GenerateValue mints a URL-safe value from crypto/rand. Length is in bytes
// of entropy before encoding, clamped to a range where the low end is still
// well past anything brute-forceable.
func GenerateValue(length int) (string, error) {
	switch {
	case length <= 0:
		length = 32
	case length < 16:
		length = 16
	case length > 128:
		length = 128
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating secret value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ─── Reads ────────────────────────────────────────────────────────────────

const vaultSelectList = `s.id, s.name, s.description, s.kind, s.group_name, s.scope, s.environment_id,
	s.stack_name, s.version, s.generated, s.system, s.origin_module,
	s.last_used_at, s.last_revealed_at, s.created_by, s.created_at, s.updated_at,
	s.auto_apply`

func scanSecret(row interface{ Scan(...any) error }) (*Secret, error) {
	var (
		sec                                   Secret
		description, groupName, environmentID sql.NullString
		stackName, originModule, lastUsedAt   sql.NullString
		lastRevealedAt, createdBy             sql.NullString
		generated, system, autoApply          int
	)
	if err := row.Scan(
		&sec.ID, &sec.Name, &description, &sec.Kind, &groupName, &sec.Scope, &environmentID,
		&stackName, &sec.Version, &generated, &system, &originModule,
		&lastUsedAt, &lastRevealedAt, &createdBy, &sec.CreatedAt, &sec.UpdatedAt,
		&autoApply,
	); err != nil {
		return nil, err
	}
	sec.Description = description.String
	if groupName.Valid && groupName.String != "" {
		group := groupName.String
		sec.Group = &group
	}
	sec.Generated = generated == 1
	sec.System = system == 1
	sec.AutoApply = autoApply == 1
	if environmentID.Valid {
		sec.EnvironmentID = &environmentID.String
	}
	if stackName.Valid {
		sec.StackName = &stackName.String
	}
	if originModule.Valid {
		sec.OriginModule = &originModule.String
	}
	if lastUsedAt.Valid {
		sec.LastUsedAt = &lastUsedAt.String
	}
	if lastRevealedAt.Valid {
		sec.LastRevealedAt = &lastRevealedAt.String
	}
	if createdBy.Valid {
		sec.CreatedBy = &createdBy.String
	}
	return &sec, nil
}

// ListFilter narrows a listing. IncludeSystem is off by default so the page
// does not open on fifty rows nobody typed.
type ListFilter struct {
	EnvironmentID string
	IncludeSystem bool
	Search        string
	// Group narrows to one group label. The sentinel GroupUngrouped selects
	// the secrets that have none, which an empty string cannot express
	// because empty already means "every group".
	Group string
}

// GroupUngrouped is the ListFilter.Group sentinel for "has no group".
const GroupUngrouped = "__ungrouped__"

const listVaultHead = `SELECT ` + vaultSelectList + `,
		(SELECT COUNT(*) FROM secret_references r WHERE r.secret_id = s.id),
		(SELECT COUNT(*) FROM secret_references r
		  WHERE r.secret_id = s.id AND r.applied_version < s.version),
		cu.username, se.name
		FROM secrets s
		LEFT JOIN users cu ON cu.id = s.created_by
		LEFT JOIN environments se ON se.id = s.environment_id
		WHERE `

const listVaultTail = `
		ORDER BY CASE WHEN s.group_name IS NULL OR s.group_name = '' THEN 1 ELSE 0 END,
		         s.group_name ASC, s.name ASC
		LIMIT ? OFFSET ?`

// List returns one page of secret metadata plus the total. Never values.
func (s *Service) List(ctx context.Context, filter ListFilter, page, perPage int) ([]Secret, int, error) {
	where := []string{"1 = 1"}
	args := []any{}

	if !filter.IncludeSystem {
		where = append(where, "s.system = 0")
	}
	if filter.EnvironmentID != "" {
		// A global secret is visible from every environment, so an
		// environment filter means "mine plus the global ones", not "mine".
		where = append(where, "(s.environment_id = ? OR s.environment_id IS NULL)")
		args = append(args, filter.EnvironmentID)
	}
	if filter.Search != "" {
		where = append(where, "s.name LIKE ?")
		args = append(args, "%"+strings.ToUpper(filter.Search)+"%")
	}
	switch filter.Group {
	case "":
	case GroupUngrouped:
		where = append(where, "(s.group_name IS NULL OR s.group_name = '')")
	default:
		where = append(where, "s.group_name = ?")
		args = append(args, filter.Group)
	}
	clause := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM secrets s WHERE "+clause, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting secrets: %w", err)
	}

	query := listVaultHead + clause + listVaultTail // SQL identifier: clause is joined from fixed "column = ?" fragments
	rows, err := s.db.QueryContext(ctx, query, append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing secrets: %w", err)
	}
	defer closeRows(rows)

	out := []Secret{}
	for rows.Next() {
		var (
			sec                                   Secret
			description, groupName, environmentID sql.NullString
			stackName, originModule, lastUsedAt   sql.NullString
			lastRevealedAt, createdBy             sql.NullString
			createdByName, environmentName        sql.NullString
			generated, system, autoApply          int
		)
		if err := rows.Scan(
			&sec.ID, &sec.Name, &description, &sec.Kind, &groupName, &sec.Scope, &environmentID,
			&stackName, &sec.Version, &generated, &system, &originModule,
			&lastUsedAt, &lastRevealedAt, &createdBy, &sec.CreatedAt, &sec.UpdatedAt,
			&autoApply, &sec.ReferenceCount, &sec.StaleCount,
			&createdByName, &environmentName,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning secret: %w", err)
		}
		sec.Description = description.String
		if groupName.Valid && groupName.String != "" {
			group := groupName.String
			sec.Group = &group
		}
		sec.Generated = generated == 1
		sec.System = system == 1
		sec.AutoApply = autoApply == 1
		if environmentID.Valid {
			sec.EnvironmentID = &environmentID.String
		}
		if stackName.Valid {
			sec.StackName = &stackName.String
		}
		if originModule.Valid {
			sec.OriginModule = &originModule.String
		}
		if lastUsedAt.Valid {
			sec.LastUsedAt = &lastUsedAt.String
		}
		if lastRevealedAt.Valid {
			sec.LastRevealedAt = &lastRevealedAt.String
		}
		if createdBy.Valid {
			sec.CreatedBy = &createdBy.String
		}
		if createdByName.Valid {
			sec.CreatedByName = &createdByName.String
		}
		if environmentName.Valid {
			sec.EnvironmentName = &environmentName.String
		}
		out = append(out, sec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating secrets: %w", err)
	}
	return out, total, nil
}

// ByID returns one secret's metadata, or ErrNotFound.
func (s *Service) ByID(ctx context.Context, id string) (*Secret, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+vaultSelectList+" FROM secrets s WHERE s.id = ?", id)
	sec, err := scanSecret(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading secret: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN applied_version < ? THEN 1 ELSE 0 END), 0)
		FROM secret_references WHERE secret_id = ?
	`, sec.Version, id).Scan(&sec.ReferenceCount, &sec.StaleCount); err != nil {
		return nil, fmt.Errorf("counting secret references: %w", err)
	}
	return sec, nil
}

// References lists what uses a secret, so a delete can warn and a rotation
// can say which stacks need a redeploy.
func (s *Service) References(ctx context.Context, secretID string) ([]SecretReference, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, secret_id, resource_type, resource_id, resource_name,
		       environment_id, field, applied_version, created_at
		FROM secret_references WHERE secret_id = ?
		ORDER BY resource_type, resource_id LIMIT 1000
	`, secretID)
	if err != nil {
		return nil, fmt.Errorf("listing secret references: %w", err)
	}
	defer closeRows(rows)

	out := []SecretReference{}
	for rows.Next() {
		var ref SecretReference
		var resourceName, environmentID, field sql.NullString
		if err := rows.Scan(&ref.ID, &ref.SecretID, &ref.ResourceType,
			&ref.ResourceID, &resourceName, &environmentID, &field,
			&ref.AppliedVersion, &ref.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret reference: %w", err)
		}
		if resourceName.Valid {
			ref.ResourceName = &resourceName.String
		}
		if environmentID.Valid {
			ref.EnvironmentID = &environmentID.String
		}
		if field.Valid {
			ref.Field = &field.String
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating secret references: %w", err)
	}
	return out, nil
}

// Versions lists a secret's rotation history — metadata only, never values.
func (s *Service) Versions(ctx context.Context, secretID string) ([]SecretVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, secret_id, version, rotated_by, created_at
		FROM secret_versions WHERE secret_id = ?
		ORDER BY version DESC LIMIT 1000
	`, secretID)
	if err != nil {
		return nil, fmt.Errorf("listing secret versions: %w", err)
	}
	defer closeRows(rows)

	out := []SecretVersion{}
	for rows.Next() {
		var v SecretVersion
		var rotatedBy sql.NullString
		if err := rows.Scan(&v.ID, &v.SecretID, &v.Version, &rotatedBy, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret version: %w", err)
		}
		if rotatedBy.Valid {
			v.RotatedBy = &rotatedBy.String
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating secret versions: %w", err)
	}
	return out, nil
}

// ─── Writes ───────────────────────────────────────────────────────────────

// Create stores a new secret. When req.Generate is set the value is minted
// server-side and returned once, in the response; it is otherwise taken from
// the request. The returned string is empty unless the value was generated —
// echoing a value the client already holds buys nothing and widens the set of
// responses that can carry a credential.
func (s *Service) Create(ctx context.Context, userID, envID string, req CreateRequest) (*Secret, string, error) {
	if err := ValidateName(req.Name); err != nil {
		return nil, "", err
	}
	if req.Scope == "" {
		req.Scope = ScopeGlobal
	}
	if req.Kind == "" {
		req.Kind = KindEnv
	}
	if err := validateScope(req.Scope); err != nil {
		return nil, "", err
	}
	req.Kind = NormalizeKind(req.Kind)
	if err := validateKind(req.Kind); err != nil {
		return nil, "", err
	}
	if err := s.requireGlobalAuthority(ctx, userID, envID, req.Scope); err != nil {
		return nil, "", err
	}

	// The scope fields must match the declared scope, or a "stack" secret
	// with no stack name would sit at the same scope_key as an environment
	// one and the unique index would reject the wrong thing.
	environmentID := emptyToNil(req.EnvironmentID)
	stackName := emptyToNil(req.StackName)
	switch req.Scope {
	case ScopeGlobal, ScopeApplication:
		environmentID, stackName = nil, nil
	case ScopeEnvironment:
		stackName = nil
		if environmentID == nil {
			return nil, "", ErrScopeInvalid
		}
	case ScopeStack:
		if stackName == nil {
			return nil, "", ErrScopeInvalid
		}
	}

	value := req.Value
	generated := false
	if req.Generate {
		minted, err := GenerateValue(req.GenerateLength)
		if err != nil {
			return nil, "", err
		}
		value = minted
		generated = true
	}
	if value == "" {
		return nil, "", ErrNameInvalid
	}

	ciphertext, err := s.enc.Encrypt(value)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting secret value: %w", err)
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	generatedFlag := 0
	if generated {
		generatedFlag = 1
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO secrets (id, name, description, kind, group_name, scope,
			environment_id, stack_name, scope_key, value, version, generated,
			system, auto_apply, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, 0, ?, ?, ?, ?)
	`, id, req.Name, req.Description, req.Kind, nullIfEmpty(NormalizeGroup(req.Group)),
		req.Scope, environmentID, stackName,
		scopeKeyFor(req.Scope, req.Name, environmentID, stackName), ciphertext, generatedFlag,
		boolToInt(req.AutoApply), nullIfEmpty(userID), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, "", ErrDuplicate
		}
		return nil, "", fmt.Errorf("inserting secret: %w", err)
	}

	sec, err := s.ByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if !generated {
		return sec, "", nil
	}
	return sec, value, nil
}

// Rotate replaces a secret's value, archiving the previous one. The
// historical value is kept so a bad rotation can be rolled back; it is never
// displayed, and only the last versionRetention are kept.
func (s *Service) Rotate(ctx context.Context, userID, envID, id string, req RotateRequest) (*Secret, string, error) {
	sec, err := s.ByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if sec.System {
		return nil, "", ErrSystemImmutable
	}
	if err := s.requireGlobalAuthority(ctx, userID, envID, sec.Scope); err != nil {
		return nil, "", err
	}

	value := req.Value
	generated := false
	if req.Generate {
		minted, err := GenerateValue(req.GenerateLength)
		if err != nil {
			return nil, "", err
		}
		value = minted
		generated = true
	}
	if value == "" {
		return nil, "", ErrNameInvalid
	}

	ciphertext, err := s.enc.Encrypt(value)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting secret value: %w", err)
	}

	if err := s.archiveAndSet(ctx, userID, sec, ciphertext); err != nil {
		return nil, "", err
	}

	rotated, err := s.ByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if !generated {
		return rotated, "", nil
	}
	return rotated, value, nil
}

// archiveAndSet moves the current ciphertext into secret_versions, writes the
// new one, bumps the version, and prunes the history. One transaction: a
// half-applied rotation would either lose the old value or leave the version
// counter lying about what is stored.
func (s *Service) archiveAndSet(ctx context.Context, userID string, sec *Secret, ciphertext string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning rotation transaction: %w", err)
	}
	defer coredb.RollbackTx(tx)

	var current string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM secrets WHERE id = ?", sec.ID).
		Scan(&current); err != nil {
		return fmt.Errorf("reading current secret value: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO secret_versions (id, secret_id, version, value, rotated_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, xid.New().String(), sec.ID, sec.Version, current, nullIfEmpty(userID), now); err != nil {
		return fmt.Errorf("archiving secret version: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE secrets SET value = ?, version = version + 1, generated = ?, updated_at = ?
		WHERE id = ?
	`, ciphertext, 0, now, sec.ID); err != nil {
		return fmt.Errorf("updating secret value: %w", err)
	}

	// Prune to the retention window. Expressed as "delete anything not in the
	// newest N" rather than a LIMIT/OFFSET delete, which MySQL rejects.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM secret_versions
		WHERE secret_id = ?
		  AND version <= (SELECT MAX(version) FROM secret_versions WHERE secret_id = ?) - ?
	`, sec.ID, sec.ID, versionRetention); err != nil {
		return fmt.Errorf("pruning secret versions: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing rotation: %w", err)
	}
	return nil
}

// Patch updates metadata only. Any change to a scope component recomputes
// scope_key through the same function every other write uses.
func (s *Service) Patch(ctx context.Context, userID, envID, id string, req PatchRequest) (*Secret, error) {
	sec, err := s.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sec.System {
		return nil, ErrSystemImmutable
	}

	scope := sec.Scope
	if req.Scope != nil {
		if err := validateScope(*req.Scope); err != nil {
			return nil, err
		}
		scope = *req.Scope
	}
	kind := sec.Kind
	if req.Kind != nil {
		normalized := NormalizeKind(*req.Kind)
		if err := validateKind(normalized); err != nil {
			return nil, err
		}
		kind = normalized
	}
	description := sec.Description
	if req.Description != nil {
		description = *req.Description
	}
	autoApply := sec.AutoApply
	if req.AutoApply != nil {
		autoApply = *req.AutoApply
	}
	group := ""
	if sec.Group != nil {
		group = *sec.Group
	}
	if req.Group != nil {
		group = NormalizeGroup(*req.Group)
	}

	environmentID := sec.EnvironmentID
	if req.EnvironmentID != nil {
		environmentID = emptyToNil(req.EnvironmentID)
	}
	stackName := sec.StackName
	if req.StackName != nil {
		stackName = emptyToNil(req.StackName)
	}
	switch scope {
	case ScopeGlobal, ScopeApplication:
		environmentID, stackName = nil, nil
	case ScopeEnvironment:
		stackName = nil
		if environmentID == nil {
			return nil, ErrScopeInvalid
		}
	case ScopeStack:
		if stackName == nil {
			return nil, ErrScopeInvalid
		}
	}

	// Both directions: promoting a secret to global needs the authority, and
	// so does touching one that already is global. Checking only the new
	// scope would let a holder of plain secrets.manage demote a global secret
	// out from under everyone who depends on it.
	if err := s.requireGlobalAuthority(ctx, userID, envID, scope); err != nil {
		return nil, err
	}
	if err := s.requireGlobalAuthority(ctx, userID, envID, sec.Scope); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		UPDATE secrets
		SET description = ?, kind = ?, group_name = ?, scope = ?, environment_id = ?,
		    stack_name = ?, scope_key = ?, auto_apply = ?, updated_at = ?
		WHERE id = ?
	`, description, kind, nullIfEmpty(group), scope, environmentID, stackName,
		scopeKeyFor(scope, sec.Name, environmentID, stackName), boolToInt(autoApply), now, id)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("updating secret: %w", err)
	}

	return s.ByID(ctx, id)
}

// Rollback restores a historical value as the current one. It does not
// display the value; the rolled-back version is archived like any rotation,
// so rolling back a rollback works.
func (s *Service) Rollback(ctx context.Context, userID, envID, id string, version int) (*Secret, error) {
	sec, err := s.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sec.System {
		return nil, ErrSystemImmutable
	}
	if err := s.requireGlobalAuthority(ctx, userID, envID, sec.Scope); err != nil {
		return nil, err
	}

	var ciphertext string
	err = s.db.QueryRowContext(ctx,
		"SELECT value FROM secret_versions WHERE secret_id = ? AND version = ?",
		id, version,
	).Scan(&ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading secret version: %w", err)
	}

	if err := s.archiveAndSet(ctx, userID, sec, ciphertext); err != nil {
		return nil, err
	}
	return s.ByID(ctx, id)
}

// Delete removes a secret. A referenced secret is a 409 unless force is set,
// and a system secret is never deletable through this path at all — it is
// owned by a host module, and removing it would break that module with no
// route back.
func (s *Service) Delete(ctx context.Context, id string, force bool) error {
	sec, err := s.ByID(ctx, id)
	if err != nil {
		return err
	}
	if sec.System {
		return ErrSystemImmutable
	}
	if sec.ReferenceCount > 0 && !force {
		return ErrInUse
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM secrets WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting secret: %w", err)
	}
	if coredb.RowsAffected(result) == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchReference records that a resource uses a secret. Idempotent through
// ref_key: re-resolving the same field on every deploy updates one row rather
// than accumulating one per deploy.
func (s *Service) TouchReference(ctx context.Context, ref SecretReference) error {
	key := refKey(ref.SecretID, ref.ResourceType, ref.ResourceID, ref.Field)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO secret_references (id, secret_id, resource_type, resource_id,
			resource_name, environment_id, field, applied_version, ref_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (ref_key) DO UPDATE SET
			resource_name = excluded.resource_name,
			environment_id = excluded.environment_id,
			applied_version = excluded.applied_version
	`, xid.New().String(), ref.SecretID, ref.ResourceType, ref.ResourceID,
		ref.ResourceName, ref.EnvironmentID, ref.Field, ref.AppliedVersion, key, now)
	if err != nil {
		return fmt.Errorf("recording secret reference: %w", err)
	}
	return nil
}

// ClearReferences drops every reference a resource holds, so a redeploy that
// stops using a secret does not leave a stale "in use" warning behind.
func (s *Service) ClearReferences(ctx context.Context, resourceType, resourceID string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM secret_references WHERE resource_type = ? AND resource_id = ?",
		resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("clearing secret references: %w", err)
	}
	return nil
}

// ─── Reveal ───────────────────────────────────────────────────────────────

// RevealRateLimit reports whether the user is under the hourly reveal cap.
//
// The counter is the audit log rather than an in-memory map: the audit row is
// written before the value is returned, so it is the authoritative record of
// what was disclosed, and counting it means the limit survives a restart. An
// in-memory limiter is cleared by exactly the action an exfiltrating operator
// with shell access would take first.
func (s *Service) RevealRateLimit(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	since := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE user_id = ? AND action = ? AND timestamp >= ?
	`, userID, RevealAuditAction, since).Scan(&count); err != nil {
		return fmt.Errorf("counting recent reveals: %w", err)
	}
	if count >= revealsPerHour {
		return ErrRateLimited
	}
	return nil
}

// Reveal returns a secret's plaintext value.
//
// This is one of exactly two places in the codebase that decrypt a vault
// value; the other is the deploy-time resolver. The handler has already
// written the audit entry by the time this runs — deliberately, so a crash or
// a dropped connection mid-response cannot produce an unlogged disclosure.
//
// A system secret is not revealable here. Those are credentials the
// application manages for itself; the owning module's own UI governs them,
// and most were write-once by design before the vault existed.
func (s *Service) Reveal(ctx context.Context, id string) (string, error) {
	var ciphertext string
	var system int
	err := s.db.QueryRowContext(ctx,
		"SELECT value, system FROM secrets WHERE id = ?", id,
	).Scan(&ciphertext, &system)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("loading secret for reveal: %w", err)
	}
	if system == 1 {
		return "", ErrSystemImmutable
	}

	plaintext, err := s.decryptValue(ciphertext, id)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		"UPDATE secrets SET last_revealed_at = ? WHERE id = ?", now, id,
	); err != nil {
		// The disclosure already happened and is already in the audit log.
		// Failing the response now would hide it from the user without
		// un-disclosing anything, so log and carry on.
		slog.Error("recording secret reveal timestamp failed", "secret_id", id, "error", err)
	}
	return plaintext, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// boolToInt writes a Go bool into the INTEGER columns SQLite uses for flags,
// which is also what the Postgres and MySQL ports of this schema expect.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueViolation recognises a duplicate-key error across the three
// supported drivers without importing any of them. Matching on message text
// is unlovely, but the alternative is a driver-specific error type in a module
// that is meant to be driver-agnostic.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || // SQLite, Postgres
		strings.Contains(msg, "duplicate key") || // Postgres
		strings.Contains(msg, "duplicate entry") // MySQL / MariaDB
}

// decryptValue is the **only** place in this module that calls
// encryption.Decrypt.
//
// §8 of the secrets plan requires that the decrypt paths be countable. They
// are: three named callers — the deploy resolver (`lookup`), the reveal
// handler (`Reveal`), and the module read-through (`SystemValue`) — all
// through this one function. `grep -n "enc.Decrypt" modules/secrets/` returns
// exactly one line, and if it ever returns two, something has grown a cache
// or an export path that needs a very hard look.
func (s *Service) decryptValue(ciphertext, secretID string) (string, error) {
	plaintext, err := s.enc.Decrypt(ciphertext)
	if err != nil {
		slog.Error("decrypting secret failed", "secret_id", secretID, "error", err)
		return "", fmt.Errorf("decrypting secret: %w", err)
	}
	return plaintext, nil
}

// coreRowsAffected wraps the db helper so adopt.go does not need the import.
func coreRowsAffected(result sql.Result) int64 { return coredb.RowsAffected(result) }

// NotifyReveal records an in-app notification for a reveal so a quiet read of
// the vault is visible to administrators rather than only to the audit log.
func (s *Service) NotifyReveal(ctx context.Context, username string, sec *Secret) error {
	if s.db == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO in_app_notifications (id, level, title, message, entity_type, entity_id, created_at, updated_at)
		VALUES (?, 'warning', ?, ?, 'secret', ?, ?, ?)
	`,
		xid.New().String(),
		"Secret revealed",
		username+" revealed the value of "+sec.Name+" ("+sec.Scope+" scope).",
		sec.ID, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert reveal notification: %w", err)
	}
	return nil
}

// EnvironmentOf reports which environment a secret belongs to ("" for a global
// secret), so a route can authorize against the secret's own environment rather
// than the one the caller named. found is false when there is no such secret.
func (s *Service) EnvironmentOf(ctx context.Context, id string) (envID string, found bool, err error) {
	var env sql.NullString
	err = s.db.QueryRowContext(ctx, "SELECT environment_id FROM secrets WHERE id = ?", id).Scan(&env)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading secret environment: %w", err)
	}
	return env.String, true, nil
}
