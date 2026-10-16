// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCopy, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { VM_PERM } from '../permissions';
import type { Share } from '../hooks/useShares';

export function SharesTable({
  shares,
  removing,
  onRemove,
}: {
  shares: Share[];
  removing: boolean;
  onRemove: (id: string) => void;
}) {
  const { t } = useTranslation('vm');

  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('shares.columns.tag')}</TableHead>
            <TableHead>{t('shares.columns.hostPath')}</TableHead>
            <TableHead>{t('shares.columns.access')}</TableHead>
            <TableHead>{t('shares.columns.mount')}</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {shares.map((share) => (
            <TableRow key={share.id}>
              <TableCell className="font-mono text-xs">{share.tag}</TableCell>
              <TableCell className="font-mono text-xs">{share.hostPath}</TableCell>
              <TableCell>
                <Badge variant={share.readOnly ? 'secondary' : 'warning'}>
                  {share.readOnly ? t('shares.readOnly') : t('shares.readWrite')}
                </Badge>
              </TableCell>
              <TableCell>
                <span className="inline-flex items-center gap-1.5">
                  <code className="font-mono text-xs">{share.mountCommand}</code>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('clipboard.copy', { ns: 'common' })}
                    onClick={() => {
                      void navigator.clipboard?.writeText(share.mountCommand ?? '');
                    }}
                  >
                    <IconCopy className="h-4 w-4" aria-hidden="true" />
                  </Button>
                </span>
              </TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon"
                  permission={VM_PERM.PRIVILEGED}
                  aria-label={t('shares.remove')}
                  disabled={removing}
                  onClick={() => onRemove(share.id)}
                >
                  <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
                </Button>
              </TableCell>
            </TableRow>
          ))}
          {shares.length === 0 && (
            <TableRow>
              <TableCell colSpan={5} className="py-6 text-center text-muted-foreground">
                {t('shares.empty')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}
