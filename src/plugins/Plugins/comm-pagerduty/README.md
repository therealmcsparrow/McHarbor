<!-- Copyright (c) 2026 McSparrow. All rights reserved. -->
<!-- McHarbor is licensed under the McHarbor License. See LICENSE for details. -->

# comm-pagerduty

PagerDuty incident channel (channel type `pagerduty`, permission
`channels.use_comm_pagerduty`). Operator documentation lives in
`.documentation/plugins/communication/pagerduty/`.

## How it talks to PagerDuty

- **Writes** go through Events API v2 (`POST /v2/enqueue`) with the service
  integration's routing key. The host's `DedupKey` is sent verbatim as
  `dedup_key`, so a retried `Open` folds into the open alert. `Ref.ExternalID`
  is the dedup key itself, because the Events API returns no incident ID.
- **Update** re-sends `trigger` on the same key; **Resolve** sends `resolve`.
- **Test** never pages: it validates the config offline and, when a REST key is
  set, calls `GET /abilities` on the region's REST host.
- **Webhooks v3** are verified against `X-PagerDuty-Signature` (any `v1=` entry,
  HMAC-SHA256 of the raw body) with `webhook_signing_secret`, falling back to the
  host's `_webhook_secret`. `event.occurred_at` becomes `RemoteStatus.UpdatedAt`.
- **Poll** needs a REST key and looks each link up with
  `GET /incidents?incident_key=…&date_range=all`, reporting only the newest
  incident. An empty result is no status, never `ErrNotFound`.
- The region picks both hosts; no base URL is configurable.

## Layout

```
backend/   main.go  schema.go  config.go  client.go  http.go
           lifecycle.go  notify.go  webhook.go  poll.go  *_test.go
frontend/  src/index.tsx  src/ConfigForm.tsx  src/SetupGuide.tsx
i18n/      de en es fr nl pt
```

## Build

```bash
cd backend && GOTOOLCHAIN=auto GOWORK=off go test ./...
cd frontend && npm ci && npm run build
```
