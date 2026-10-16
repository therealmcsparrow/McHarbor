// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconShieldCheck } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { useVerifyAuditChain, type AuditVerifyResult } from '../hooks/useAuditCenter';

type AuditVerifyDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function verdict(result: AuditVerifyResult): { variant: 'success' | 'warning' | 'destructive'; key: string } {
  if (!result.keyConfigured) return { variant: 'destructive', key: 'verify.keyMissing' };
  if (!result.verified) return { variant: 'destructive', key: 'verify.broken' };
  if (!result.complete) return { variant: 'warning', key: 'verify.incomplete' };
  return { variant: 'success', key: 'verify.verified' };
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md border border-border bg-background/40 px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-lg font-semibold text-foreground">{value}</p>
    </div>
  );
}

function VerifyResultView({ result }: { result: AuditVerifyResult }) {
  const { t } = useTranslation('audit');
  const { variant, key } = verdict(result);
  const problems = [
    result.firstBrokenId ? t('verify.firstBroken', { id: result.firstBrokenId }) : '',
    result.reason ? t('verify.reason', { reason: result.reason }) : '',
    result.truncated ? t('verify.truncated', { count: result.checkedRows }) : '',
  ].filter(Boolean);

  return (
    <div className="space-y-4">
      <Alert
        variant={variant}
        title={t(key)}
        description={problems.join(' ') || t('verify.checkedAt', { time: formatDate(result.checkedAt) })}
      />
      <div className="grid gap-3 sm:grid-cols-3">
        <Stat label={t('verify.checked')} value={result.checkedRows} />
        <Stat label={t('verify.hashed')} value={result.hashedRows} />
        <Stat label={t('verify.unsealed')} value={result.unsealedRows} />
        <Stat label={t('verify.prunedGaps')} value={result.prunedGaps} />
        <Stat label={t('verify.prunedRows')} value={result.prunedRows} />
        <Stat label={t('verify.invalidTombstones')} value={result.invalidTombstones} />
      </div>
      <div className="space-y-2">
        <p className="text-xs font-medium tracking-wider text-muted-foreground uppercase">{t('verify.anchors')}</p>
        {result.anchors.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('verify.noAnchors')}</p>
        ) : (
          <ul className="space-y-1">
            {result.anchors.map((anchor) => (
              <li
                key={anchor.id}
                className="rounded-md border border-border bg-background/40 px-3 py-2 text-sm text-foreground"
              >
                <p>{t('verify.anchorBy', { actor: anchor.actor, time: formatDate(anchor.timestamp) })}</p>
                <p className="font-mono text-xs break-all text-muted-foreground">{anchor.details}</p>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

export function AuditVerifyDialog({ open: isOpen, onOpenChange }: AuditVerifyDialogProps) {
  const { t } = useTranslation('audit');
  const verify = useVerifyAuditChain();

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl p-0">
        <DialogHeader>
          <DialogTitle>{t('verify.title')}</DialogTitle>
          <DialogDescription>{t('verify.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {verify.isPending && (
            <output className="flex items-center gap-2 text-sm text-muted-foreground">
              <Spinner size="sm" />
              {t('verify.running')}
            </output>
          )}
          {verify.isError && (
            <Alert variant="destructive" title={t('verify.failed')} description={verify.error.message} />
          )}
          {verify.data && !verify.isPending && <VerifyResultView result={verify.data} />}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('actions.close', { ns: 'common' })}
          </Button>
          <Button permission={PERM.AUDIT_VIEW} onClick={() => verify.mutate()} disabled={verify.isPending}>
            {verify.isPending ? <Spinner size="sm" /> : <IconShieldCheck className="size-4" />}
            {verify.data ? t('verify.rerun') : t('verify.run')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
