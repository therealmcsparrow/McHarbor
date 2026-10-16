<!-- Copyright (c) 2026 McSparrow. All rights reserved. -->
<!-- McHarbor is licensed under the McHarbor License. See LICENSE for details. -->

# comm-zammad

Opens, notes and closes Zammad tickets from McHarbor alerts through the Zammad REST API
(`/api/v1`, access-token auth), and reads ticket state back from HMAC-signed trigger
webhooks or by polling. Channel type `zammad`, permission key `channels.use_comm_zammad`.

## How it maps

- **Dedup** — Zammad has no correlation field. Each firing gets the title marker
  `[MH-<10 hex>]` (SHA-256 of `DedupKey|OccurredAt`) and the tag `mcharbor`. `Open`
  searches for the marker and accepts only a ticket whose title starts with it and whose
  state is not `closed` or `merged`, so the fuzzy database search without Elasticsearch
  cannot pick the wrong ticket. A failed tag call is retried by the outbox through `Open`.
- **Update** — an internal note (`POST /api/v1/ticket_articles`).
- **Resolve** — one `PUT /api/v1/tickets/{id}` that sets `resolve_state` and adds the note.
- **Webhook** — `X-Hub-Signature: sha1=<hex>` over the raw body, keyed with the channel's
  inbound secret, compared in constant time. Only tickets whose title starts with `[MH-`
  produce a status.
- **Poll** — `GET /api/v1/tickets/{id}?expand=true`, four at a time; a 404 is omitted.

## Layout

```
backend/   main.go  schema.go  config.go  http.go  mapping.go  client.go  webhook.go  poll.go
           http_test.go  client_test.go  webhook_test.go
frontend/  src/index.tsx  src/ConfigForm.tsx  src/SetupGuide.tsx  src/useZammadValidation.ts
i18n/      de en es fr nl pt
```

## Build

```bash
cd backend && GOTOOLCHAIN=auto GOWORK=off go test ./...
cd frontend && npm ci && npm run build
```
