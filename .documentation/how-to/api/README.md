# Script the API

Automating McHarbor from outside: getting a credential, calling endpoints, handling the
envelope, paging, streaming, and failing safely.

The [API reference](../../api/README.md) documents every endpoint. This page is the practical
loop around it.

## 1. Get a credential

For anything unattended, use an **API key**. A session cookie is subject to the idle timeout,
which is driven by browser activity rather than by requests. A script using a cookie will be
signed out mid-run even while it is making calls.

```bash
BASE=http://localhost:8705

# Sign in once, interactively, to mint the key
curl -sc jar.txt -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"…"}'

# What may this account put on a key?
curl -sb jar.txt "$BASE/api/api-keys/options"

# Mint it
curl -sb jar.txt -X POST "$BASE/api/api-keys" \
  -H 'Content-Type: application/json' \
  -d '{"name":"ci","roleIds":[],"scopes":["containers.view","containers.manage"],"environmentIds":["<envId>"]}'
```

The plaintext key is in the response **once**. Store it then; only a hash is kept.

```bash
export MCH_KEY=mch_…
curl -s -H "Authorization: Bearer $MCH_KEY" "$BASE/api/containers?env=<envId>"
```

Two properties to design around:

- **A key cannot outrank its owner.** Its rights are the intersection of what it was granted
  and what the owner still holds, so demoting the owner narrows every key they issued. Do not
  mint automation keys from a personal account that will change roles.
- **Scope it.** Grant the environments and permissions the job actually needs. `options` tells
  you what is available; it is usually smaller than the full catalog.

## 2. Always name the environment

```bash
curl -s -H "Authorization: Bearer $MCH_KEY" "$BASE/api/containers?env=$ENV_ID"
```

Omitting `?env=` uses whichever environment currently carries the default flag. That is fine
on a single-host install and a latent bug on any other, because the answer changes when an
operator moves the flag. Pass it explicitly in every script.

`?env=` is also the scope the permission check uses, so the same path can answer 200 on one
environment and 403 on another.

## 3. Read the envelope, branch on `code`

```json
{ "success": false, "error": "Container not found", "code": "err.container.not_found" }
```

`error` is translated into the request's language and its wording is not a contract. `code`
is stable. Branch on `code`.

```bash
resp=$(curl -s -H "Authorization: Bearer $MCH_KEY" "$BASE/api/containers/$ID")
if [ "$(jq -r .success <<<"$resp")" != "true" ]; then
  case "$(jq -r .code <<<"$resp")" in
    err.container.not_found) echo "gone"; exit 0 ;;
    err.api_key.expired)     echo "rotate the key"; exit 2 ;;
    *)                       jq -r .error <<<"$resp" >&2; exit 1 ;;
  esac
fi
jq .data <<<"$resp"
```

The three key failures are worth handling separately: `err.api_key.invalid` means the key is
wrong, `err.api_key.revoked` means it used to work, `err.api_key.expired` means rotate. Retry
logic differs for each.

Every code is listed in [Errors](../../api/conventions/errors.md).

## 4. Page properly

```bash
page=1
while :; do
  body=$(curl -s -H "Authorization: Bearer $MCH_KEY" \
    "$BASE/api/audit?page=$page&per_page=500&from=2026-01-01T00:00:00Z")
  jq -c '.data.items[]' <<<"$body"
  [ "$page" -ge "$(jq -r .data.total_pages <<<"$body")" ] && break
  page=$((page + 1))
done
```

The default page size is 25 and the maximum is 1000. **A larger value is ignored, not
rejected**: ask for 5000 and you silently get 25, which looks like missing data rather than
a bad request.

Not every list paginates. Small bounded collections return a plain array in `data`, and
"most recent N" endpoints take `?limit=` instead.

## 5. Consume a stream

```bash
curl -sN -H "Authorization: Bearer $MCH_KEY" "$BASE/api/events/stream?env=$ENV_ID"
```

`-N` disables buffering; without it nothing appears until the stream ends. The first frame is
always `event: connected`, so a client can tell "open and quiet" from "still connecting".

If McHarbor sits behind a reverse proxy, that proxy must not buffer these responses. nginx
needs buffering off on the `/api` location or SSE does not work at all.

Every streaming endpoint is listed in [Streaming](../../api/conventions/streaming.md).

## 6. Poll what does not stream

Not everything slow streams. A backup or restore run returns immediately and is polled:

```bash
run=$(curl -s -H "Authorization: Bearer $MCH_KEY" \
  -X POST "$BASE/api/container-backups/$PLAN_ID/run" | jq -r .data.id)

until [ "$(curl -s -H "Authorization: Bearer $MCH_KEY" \
      "$BASE/api/container-backups/runs/$run" | jq -r .data.status)" != "running" ]; do
  sleep 5
done
```

The run endpoint is a plain read with no audit entry, precisely so it can be polled.

## 7. Respect the confirmations

Some mutations require an explicit acknowledgement in the **body**, so a copied command line
cannot do damage on its own:

| Endpoint | Requires |
|---|---|
| `POST /api/metrics/host/prune` | `confirm: true`, and `volumes: true` before named volumes are deleted |
| `POST /api/system/reset` | A specific double-confirmation string |
| `POST /api/system/os-updates/apply` | `confirm: true` |

Container removal is the other place to be careful: the extended remove takes four separate
opt-ins, and an anonymous volume holding a database is still a volume.

## 8. Things that will bite

**Rate limiting is on the auth group only** (login, setup, password reset, SSO callbacks), at
10 requests per minute. A script that logs in per request will hit it. Log in once, or use a
key.

**The JSON body limit is 8 MiB.** Multipart uploads are not bound by it.

**A failed test is a 200.** Connection tests for storage, channels, AI providers and identity
providers return success with a failing step report. Check the report, not the status code.

**`completed` is not "everything worked"** for a workflow run: a failed node does not fail the
run by default.

**Purges need `?vacuum=true`** to actually return disk space; without it SQLite frees the
pages internally and the file does not shrink.

## 9. Discovering endpoints

`GET /api/docs` returns the OpenAPI document for the whole API as JSON, useful for generating
a client. The reference here is the annotated version, with the permission each route needs
and the reasons behind the shapes.

## Related

- [API](../../api/README.md): the whole reference
- [Authentication](../../api/conventions/authentication.md): every credential in detail
- [Parameters](../../api/conventions/parameters.md): `?env=`, paging, filters
- [Errors](../../api/conventions/errors.md): every code
- [Streaming](../../api/conventions/streaming.md): SSE and WebSocket
