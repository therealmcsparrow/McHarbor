<!-- Copyright (c) 2026 McSparrow. All rights reserved. -->
<!-- McHarbor is licensed under the McHarbor License. See LICENSE for details. -->

# comm-otrs

ITSM channel for OTRS, Znuny and OTOBO (channel type `otrs`). It opens, notes and closes
tickets through a GenericInterface REST web service and reads their state back by invoker
webhook or polling. Operator documentation: `.documentation/plugins/communication/otrs/`.

## Layout

```
backend/     main.go (registration)  client.go (config, SSRF checks)  auth.go (session cache, re-login)
             http.go (transport, HTTP-200 error envelope)  lifecycle.go (Open/Update/Resolve/Send)
             probe.go (Test)  webhook.go (VerifyWebhook/ParseWebhook)  poll.go  mapping.go
             *_test.go, testdata/ (invoker payload fixtures)
webservice/  McHarborConnectorREST.yml (Znuny/OTRS)  McHarborConnectorREST-otobo.yml
frontend/    src/ConfigForm.tsx  src/SetupGuide.tsx  src/fields.ts  src/index.tsx
i18n/        de en es fr nl pt
```

## Build

```bash
cd backend && GOTOOLCHAIN=auto GOWORK=off go test ./...
cd frontend && npm ci && npm run build
```
