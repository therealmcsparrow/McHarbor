<!-- Copyright (c) 2026 McSparrow. All rights reserved. -->
<!-- McHarbor is licensed under the McHarbor License. See LICENSE for details. -->

# comm-itop

Creates, logs and resolves Combodo iTop Incidents (or User Requests) from McHarbor alerts
through the iTop REST/JSON web service (`/webservices/rest.php?version=1.3`), and reads
their status back through the Combodo Webhook Integration or by polling.

Operator documentation: `.documentation/plugins/communication/itop/`.

## Layout

```
backend/   main.go  schema.go  config.go (parseConfig, severity table)
           rest.go (rest.php client, oqlQuote, sanitize)  client.go (Test, Send, Open, Update, Resolve)
           webhook.go (VerifyWebhook, ParseWebhook, Poll)  mapping.go (marker, status map, describe)
frontend/  src/index.tsx  ConfigForm.tsx  LifecycleFields.tsx  SetupGuide.tsx  fields.tsx  useItopValidation.ts
i18n/      de en es fr nl pt — all six ship
```

## Build

```bash
cd backend && GOTOOLCHAIN=auto GOWORK=off go test ./...
cd frontend && npm ci && npm run build
```
