# API key rotation

**Security → Management → API keys**

## Rotate key

The rotate action on an active key's row (`api_keys.manage`, and the same ownership rule as
edit and revoke) issues a replacement and revokes the original **in one transaction**, so there is
never a moment with both secrets or with neither. The replacement keeps the original's owner,
name, roles, additional permissions and environment fence, and its **lifetime**: a 90-day key
rotated on day 80 gets 90 fresh days. The new secret is shown **once**, in the same dialog a new
key uses. Revoked and deleted keys cannot be rotated (409, reason `not_rotatable`).

`POST /api/api-keys/{id}/rotate` → 201 with the new key and its plaintext. Audit: `rotate` on
the old key with `replaced_by=<new id>`.

## Rotation reminders

The list flags active keys:

| Badge | When |
|---|---|
| **Rotate** | older than 90 days (hover for the age) |
| **Expiring soon** | expires within 14 days |

The thresholds are request parameters: `GET /api/api-keys?rotateAfterDays=&expiringWithinDays=`.
The fields `ageDays`, `rotationDue` and `expiringSoon` are computed per request and never stored.

## Alerts

Two platform alert conditions (**Alerts → New rule → Platform**) watch the same thing without
anyone opening the page:

| Condition | Value | Default |
|---|---|---|
| **API key due for rotation** (`api_key_rotation_due`) | age of each active key, in days | `>= 90` |
| **API key expiring** (`api_key_expiring`) | days until expiry, for active keys that have one | `<= 14` |

The subject is the key, so rotating or revoking it resolves the alert.
