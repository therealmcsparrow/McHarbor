// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconShieldCheck, IconShieldOff } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Label } from '@resources/components/ui/Label';
import { cn } from '@resources/utils/cn';
import type { PluginVersion } from '../types/plugins';

/**
 * A plugin release is "unsigned" when the catalog carries no ed25519
 * signature for it — the ordinary state for a third-party plugin the
 * project never signed. Unsigned plugins install once the operator
 * confirms; the host's job is to say so plainly rather than to refuse
 * or to stay quiet.
 */
export function isUnsignedVersion(version: PluginVersion | null | undefined): boolean {
  return !version?.signature;
}

/**
 * A plugin author, split into the parts the UI treats differently.
 *
 * Catalog entries write the author the way git does —
 * `McSparrow <info@mcsparrow.eu>`. The store shows the name and never
 * the address as text; the address, when there is one, becomes the
 * href of a mailto link on that name.
 *
 * An author field that is only an address degrades to its local part
 * for the name rather than rendering an empty line.
 */
export function parseAuthor(author: string): { name: string; email: string } {
  const email = author.match(/[^\s<(,;]+@[^\s>),;]+\.[^\s>),;]+/)?.[0] ?? '';
  const itemName = author
    .replace(/<[^>]*>/g, ' ')
    .replace(/\(([^)]*@[^)]*)\)/g, ' ')
    .replace(/\S+@\S+\.\S+/g, ' ')
    .replace(/\s+/g, ' ')
    // A removed address leaves the separator that introduced it behind:
    // "Bob <b@c.d>, Team" must not render as "Bob , Team".
    .replace(/\s+([,;])/g, '$1')
    .replace(/^[\s,;:-]+|[\s,;:-]+$/g, '');
  if (itemName) return { name: itemName, email };
  return { name: email.split('@')[0] ?? '', email };
}

/**
 * Signature state for one plugin release, as a single icon.
 *
 * Both states render. A mark that only appears when something is wrong
 * reads as missing information rather than as reassurance, and the card
 * reserves this slot either way.
 *
 * The label is not dropped with the chip — it moves to `aria-label` and
 * `title`, so the state is still named on hover and to a screen reader.
 * An icon whose meaning lives only in its colour is not a state anyone
 * can act on.
 */
export function PluginSignatureIcon({ signed, className }: { signed: boolean; className?: string }) {
  const { t } = useTranslation('plugins');
  const label = signed ? t('signature.signedTooltip') : t('signature.unsignedTooltip');
  const Icon = signed ? IconShieldCheck : IconShieldOff;
  return (
    <span title={label} className={cn('inline-flex items-center', signed ? 'text-success' : 'text-warning', className)}>
      <Icon className="size-4" aria-hidden="true" />
      <span className="sr-only">{label}</span>
    </span>
  );
}

interface PluginUnsignedWarningProps {
  acknowledged: boolean;
  onAcknowledgedChange: (next: boolean) => void;
  pluginName?: string;
  refusedByServer?: boolean;
}

/**
 * Pre-install warning for an unsigned plugin, with the acknowledgement
 * the Install button waits on. The backend refuses an unsigned install
 * that does not carry this consent as allow_unsigned, so this is the
 * moment where the operator takes the risk knowingly.
 */
export function PluginUnsignedWarning({
  acknowledged,
  onAcknowledgedChange,
  pluginName,
  refusedByServer,
}: PluginUnsignedWarningProps) {
  const { t } = useTranslation('plugins');
  return (
    <Alert variant="warning" title={t('unsigned.title')} description={t('unsigned.description')}>
      {refusedByServer && <p className="mt-2 text-xs">{t('unsigned.serverUnverified')}</p>}
      {pluginName && <p className="mt-2 text-xs font-medium">{t('unsigned.hostPrivileges', { name: pluginName })}</p>}
      <div className="mt-3 flex items-start gap-2">
        <Checkbox
          id="plugin-unsigned-ack"
          checked={acknowledged}
          onCheckedChange={(next) => onAcknowledgedChange(next === true)}
        />
        <Label htmlFor="plugin-unsigned-ack" className="text-xs leading-snug font-normal">
          {t('unsigned.acknowledge')}
        </Label>
      </div>
    </Alert>
  );
}
