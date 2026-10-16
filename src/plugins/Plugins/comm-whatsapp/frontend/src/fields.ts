// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// comm-whatsapp — field catalogue for the four connection methods.
// Required + type mirror the backend's whatsappConfigSchema so the
// host can render the form without the bundle.

import { IconCloud, IconServer, IconBrandWindows, IconPlug } from '@tabler/icons-react';
import type { WhatsappMethod } from './index';

export type ConfigFieldType = 'string' | 'secret' | 'bool';

export type ConfigField = {
  key: string;
  labelKey: string;
  type: ConfigFieldType;
  required: boolean;
  helpKey?: string;
  placeholder?: string;
  defaultValue?: string;
};

export const METHOD_OPTIONS: { value: WhatsappMethod; labelKey: string; subKey: string; icon: typeof IconCloud }[] = [
  { value: 'cloud', labelKey: 'form.methods.cloud.label', subKey: 'form.methods.cloud.sub', icon: IconCloud },
  { value: 'gateway', labelKey: 'form.methods.gateway.label', subKey: 'form.methods.gateway.sub', icon: IconServer },
  { value: 'bot', labelKey: 'form.methods.bot.label', subKey: 'form.methods.bot.sub', icon: IconBrandWindows },
  { value: 'saas', labelKey: 'form.methods.saas.label', subKey: 'form.methods.saas.sub', icon: IconPlug },
];

export const RECIPIENT_FIELD: ConfigField = {
  key: 'recipient_phone',
  labelKey: 'form.fields.recipient_phone.label',
  type: 'string',
  required: true,
  placeholder: '+14155551234',
  helpKey: 'form.fields.recipient_phone.help',
};

const CLOUD_FIELDS: ConfigField[] = [
  {
    key: 'phone_number_id',
    labelKey: 'form.fields.phone_number_id.label',
    type: 'string',
    required: true,
    placeholder: '123456789012345',
    helpKey: 'form.fields.phone_number_id.help',
  },
  {
    key: 'access_token',
    labelKey: 'form.fields.access_token.label',
    type: 'secret',
    required: true,
    placeholder: 'EAAB...',
    helpKey: 'form.fields.access_token.help',
  },
];

const GATEWAY_FIELDS: ConfigField[] = [
  {
    key: 'gateway_url',
    labelKey: 'form.fields.gateway_url.label',
    type: 'string',
    required: true,
    placeholder: 'https://wa-gateway.local:8443',
    helpKey: 'form.fields.gateway_url.help',
  },
  {
    key: 'gateway_token',
    labelKey: 'form.fields.gateway_token.label',
    type: 'secret',
    required: true,
    helpKey: 'form.fields.gateway_token.help',
  },
];

const BOT_FIELDS: ConfigField[] = [
  {
    key: 'business_api_url',
    labelKey: 'form.fields.business_api_url.label',
    type: 'string',
    required: true,
    placeholder: 'https://waba.360dialog.com/v1/messages',
    helpKey: 'form.fields.business_api_url.help',
  },
  {
    key: 'api_key',
    labelKey: 'form.fields.api_key.label',
    type: 'secret',
    required: true,
    helpKey: 'form.fields.api_key.help',
  },
];

const SAAS_FIELDS: ConfigField[] = [
  {
    key: 'provider_url',
    labelKey: 'form.fields.provider_url.label',
    type: 'string',
    required: true,
    placeholder: 'https://api.twilio.com/2010-04-01',
    helpKey: 'form.fields.provider_url.help',
  },
  {
    key: 'provider_api_key',
    labelKey: 'form.fields.provider_api_key.label',
    type: 'secret',
    required: true,
    helpKey: 'form.fields.provider_api_key.help',
  },
  {
    key: 'provider_from',
    labelKey: 'form.fields.provider_from.label',
    type: 'string',
    required: true,
    placeholder: 'whatsapp:+14155550000',
    helpKey: 'form.fields.provider_from.help',
  },
];

export const ALL_METHOD_FIELDS: ConfigField[] = [...CLOUD_FIELDS, ...GATEWAY_FIELDS, ...BOT_FIELDS, ...SAAS_FIELDS];

export function fieldsFor(method: WhatsappMethod): ConfigField[] {
  switch (method) {
    case 'cloud':
      return CLOUD_FIELDS;
    case 'gateway':
      return GATEWAY_FIELDS;
    case 'bot':
      return BOT_FIELDS;
    case 'saas':
      return SAAS_FIELDS;
  }
}
