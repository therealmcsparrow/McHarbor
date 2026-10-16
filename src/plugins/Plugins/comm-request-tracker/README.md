<!-- Copyright (c) 2026 McSparrow. All rights reserved. -->
<!-- McHarbor is licensed under the McHarbor License. See LICENSE for details. -->

# comm-request-tracker

Creates, comments on and resolves Best Practical Request Tracker (RT) tickets from McHarbor
alerts through REST2 (`/REST/2.0/`, RT 5.0 and 6.0), authenticated with an RT auth token.
Channel type `request_tracker`; routing is gated by `channels.use_comm_request_tracker`.

- **Dedup** — a queue-applied custom field (default `McHarbor Dedup Key`) holds the SHA-256 hex
  of the host's dedup key. `Open` searches `Queue = … AND 'CF.{…}' = … AND Status = '__Active__'`
  before creating, and re-reads a new ticket to prove RT stored the value.
- **Lifecycle** — repeats are internal comments (`/comment`, never `/correspond`); recovery is
  one `/comment` call that also sets `resolve_status`.
- **Status back** — RT has no webhook, so the operator installs the Scrip in
  `assets/mcharbor-scrip.pl` with the secret and URL in `RT_SiteConfig.d/McHarbor.pm`
  (`assets/McHarbor.pm`). It signs `X-McHarbor-Signature: sha256=<hex HMAC-SHA256(secret, ts + "." + body)>`
  with `X-McHarbor-Timestamp`. Without it, the host polls through `Poll`.

Operator documentation: `.documentation/plugins/communication/request-tracker/`.

## Layout

```
backend/   main.go  schema.go  config.go (validation, tables, hashKey, TicketSQL quoting)
           rest2.go (do, status classification)  client.go (Test, Send, Open, Update, Resolve)
           webhook.go (Scrip callout)  poll.go  *_test.go
frontend/  src/index.tsx  ConfigForm.tsx  SetupGuide.tsx  ScripSnippet.tsx  useRtValidation.ts  scrip.ts
assets/    mcharbor-scrip.pl  McHarbor.pm
i18n/      de en es fr nl pt
```

## Build

```bash
cd backend && GOTOOLCHAIN=auto GOWORK=off go test ./...
cd frontend && npm ci && npm run build
```
