// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// comm-signal ConfigForm — 4-button method selector + per-method
// field subsets. Method keys stay in lockstep with backend's
// methodRest / methodBot / methodSignald / methodSimple constants
// — renaming a value here requires a Go change too.
//
// Fully controlled: takes `value` and `onChange` from the host;
// every keystroke, method swap, and field clear flows through
// onChange immediately. Mirrors comm-telegram's pattern so the
// host's PluginFormOutlet (which pushes value back via update())
// stays the single source of truth.

import { useTranslation } from 'react-i18next';
import { IconCloud, IconRobot, IconBrandDocker, IconPlugConnected, IconInfoCircle } from '@tabler/icons-react';

export type SignalMethod = 'rest' | 'bot' | 'signald' | 'simple';

type FieldKey =
  | 'server_url'
  | 'sender_number'
  | 'recipients'
  | 'auth_username'
  | 'auth_password'
  | 'bot_token'
  | 'account_id';

// Field subsets per method — shared with backend (and the Go
// config schema) so a method swap clears stale credentials
// immediately rather than leaving them on the channel row.
const COMMON_FIELDS: FieldKey[] = ['server_url', 'sender_number', 'recipients'];
const REST_FIELDS: FieldKey[] = [...COMMON_FIELDS, 'auth_username', 'auth_password'];
const BOT_FIELDS: FieldKey[] = [...COMMON_FIELDS, 'bot_token'];
const SIGNALD_FIELDS: FieldKey[] = [...COMMON_FIELDS, 'account_id'];
const SIMPLE_FIELDS: FieldKey[] = [...COMMON_FIELDS];

const FIELDS_BY_METHOD: Record<SignalMethod, FieldKey[]> = {
  rest: REST_FIELDS,
  bot: BOT_FIELDS,
  signald: SIGNALD_FIELDS,
  simple: SIMPLE_FIELDS,
};

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, message: string | null) => void;
  readOnlyFields?: string[];
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

export function ConfigForm({
  value,
  onChange,
  onError: _onError,
  readOnlyFields: _readOnlyFields = [],
  t: externalT,
}: Props) {
  // Host's i18next instance first, fall back to the local bundle's
  // comm-signal namespace. Same fallback chain the host uses for
  // other plugins (see PluginFormOutlet's makeT).
  const localT = useTranslation('comm-signal').t;
  const t = externalT ?? localT;

  // Default to 'rest' if nothing is set — signal-cli-rest-api is
  // the most common home-server option, matching the dialog in the
  // reference image where it's the highlighted first tile.
  const method: SignalMethod = (value.method as SignalMethod) || 'rest';

  function pickMethod(next: SignalMethod) {
    if (next === method) return;
    // Strip every method-specific field so stale credentials from
    // the previous method don't leak into the new one. Common
    // fields (server_url / sender_number / recipients) are kept.
    const keep = new Set<string>(FIELDS_BY_METHOD[next]);
    const cleared: Record<string, string> = {};
    for (const [k, v] of Object.entries(value)) {
      if (keep.has(k)) cleared[k] = v;
    }
    cleared.method = next;
    onChange(cleared);
  }

  function update(key: string, v: string) {
    onChange({ ...value, [key]: v });
  }

  const visibleFields = FIELDS_BY_METHOD[method];

  return (
    <div className="space-y-6">
      {/* Method selector — four tiles mirroring the reference image */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <MethodTile
          active={method === 'rest'}
          onClick={() => pickMethod('rest')}
          icon={<IconCloud className="size-5" />}
          title={t('methods.rest')}
          subtitle={t('methods.restSub')}
        />
        <MethodTile
          active={method === 'bot'}
          onClick={() => pickMethod('bot')}
          icon={<IconRobot className="size-5" />}
          title={t('methods.bot')}
          subtitle={t('methods.botSub')}
        />
        <MethodTile
          active={method === 'signald'}
          onClick={() => pickMethod('signald')}
          icon={<IconBrandDocker className="size-5" />}
          title={t('methods.signald')}
          subtitle={t('methods.signaldSub')}
        />
        <MethodTile
          active={method === 'simple'}
          onClick={() => pickMethod('simple')}
          icon={<IconPlugConnected className="size-5" />}
          title={t('methods.simple')}
          subtitle={t('methods.simpleSub')}
        />
      </div>

      {/* Field renderer — shared so each method's subset looks the same */}
      <div className="space-y-4">
        {visibleFields.map((key) => (
          <FieldRenderer key={key} fieldKey={key} value={value[key] ?? ''} onChange={(v) => update(key, v)} t={t} />
        ))}
      </div>

      {/* Hint strip — keeps the user oriented when they switch methods */}
      <p className="flex items-start gap-2 text-xs text-muted-foreground">
        <IconInfoCircle className="mt-0.5 size-3.5 shrink-0" />
        <span>{t(`help.${method}`)}</span>
      </p>
    </div>
  );
}

function MethodTile({
  active,
  onClick,
  icon,
  title,
  subtitle,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  title: string;
  subtitle: string;
}) {
  return (
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- this bundle has no @resources alias (see vite-plugin.config.ts), so Button/PlainButton cannot be imported
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={
        'rounded-lg border p-4 text-left transition-colors ' +
        (active
          ? 'border-primary bg-primary/10 ring-2 ring-primary/40'
          : 'border-border bg-card hover:border-primary/40 hover:bg-card/80')
      }
    >
      <div className="flex items-center gap-2">
        <span className="text-primary">{icon}</span>
        <span className="font-medium text-foreground">{title}</span>
      </div>
      <div className="mt-1 text-xs text-muted-foreground">{subtitle}</div>
    </button>
  );
}

function FieldRenderer({
  fieldKey,
  value,
  onChange,
  t: externalT,
}: {
  fieldKey: FieldKey;
  value: string;
  onChange: (next: string) => void;
  t: (key: string, vars?: Record<string, unknown>) => string;
}) {
  // FieldRenderer is a top-level function in the bundle, so it
  // doesn't see ConfigForm's closure variables. Pass `t` down
  // explicitly. Fall back to the bundle's own useTranslation if
  // the parent didn't supply one (the host's PluginFormOutlet
  // always does — the fallback keeps the bundle renderable in
  // standalone tests).
  const localT = useTranslation('comm-signal').t;
  const t = externalT ?? localT;
  const meta = FIELD_META[fieldKey];

  return (
    <label className="block space-y-1">
      <span className="text-sm font-medium text-foreground">
        {t(`fields.${fieldKey}`)}
        {meta.required ? <span className="ml-1 text-destructive">*</span> : null}
      </span>
      <input
        type={meta.secret ? 'password' : 'text'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={meta.placeholder}
        autoComplete="off"
        spellCheck={false}
        className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/40 focus:outline-none"
      />
      <span className="block text-xs text-muted-foreground">{t(`help.${fieldKey}`)}</span>
    </label>
  );
}

const FIELD_META: Record<FieldKey, { required: boolean; secret: boolean; placeholder: string }> = {
  server_url: { required: true, secret: false, placeholder: 'https://signal.example.com' },
  sender_number: { required: true, secret: false, placeholder: '+1234567890' },
  recipients: { required: true, secret: false, placeholder: '+1234567890, +0987654321' },
  auth_username: { required: false, secret: false, placeholder: '' },
  auth_password: { required: false, secret: true, placeholder: '' },
  bot_token: { required: true, secret: true, placeholder: '' },
  account_id: { required: true, secret: false, placeholder: 'uuid-or-phone' },
};
