// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const SCRIP_PREPARE = 'return 1;';

export const SCRIP_COMMIT = `my $t = $self->TicketObj;
my ($cf) = @{ $t->CustomFieldValues('McHarbor Dedup Key')->ItemsArrayRef };
return 1 unless $cf;
require LWP::UserAgent; require JSON; require Digest::SHA;
my $body = JSON::encode_json({ id => $t->Id + 0, status => $t->Status,
  old_status => $self->TransactionObj->OldValue, owner => $t->OwnerObj->Name,
  queue => $t->QueueObj->Name, dedup_key => $cf->Content,
  updated => $t->LastUpdatedObj->W3CDTF });
my $ts = time();
my $sig = Digest::SHA::hmac_sha256_hex("$ts.$body", RT->Config->Get('McHarborWebhookSecret'));
LWP::UserAgent->new(timeout => 10)->post(RT->Config->Get('McHarborWebhookURL'),
  'Content-Type' => 'application/json', 'X-McHarbor-Timestamp' => $ts,
  'X-McHarbor-Signature' => "sha256=$sig", Content => $body);
return 1;`;

export const SITE_CONFIG = `Set($McHarborWebhookSecret, 'paste-the-inbound-secret-here');
Set($McHarborWebhookURL, 'https://mcharbor.example.com/api/incident-webhooks/paste-the-token-here');
1;`;
