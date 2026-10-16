// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { useLeases } from '../hooks/useCompute';
import { formatAbsolute } from '../format';
import type { Network } from '../types';

/**
 * The addresses a managed network has handed out.
 *
 * The reported name column is the one to be careful with: it is whatever the
 * guest asked to be called, so it is rendered as plain text inside a cell and
 * nothing else. It is never a link, never an attribute value, and never used to
 * identify the guest — the guest id column is what does that, and it comes from
 * McHarbor rather than from the guest.
 */
export function LeaseViewer({ network, onClose }: { network: Network; onClose: () => void }) {
  const { t } = useTranslation('compute');
  const { data, isLoading } = useLeases(network.id);

  const leases = data ?? [];

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('networks.leases.title')}</DialogTitle>
          <DialogDescription>
            {network.name} · {network.subnetCidr}
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : leases.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('networks.leases.empty')}</p>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('networks.leases.columns.ip')}</TableHead>
                  <TableHead>{t('networks.leases.columns.mac')}</TableHead>
                  <TableHead>{t('networks.leases.columns.hostname')}</TableHead>
                  <TableHead>{t('networks.leases.columns.guest')}</TableHead>
                  <TableHead>{t('networks.leases.columns.expires')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {leases.map((lease) => (
                  <TableRow key={lease.id}>
                    <TableCell>
                      <code className="font-mono text-xs text-foreground">{lease.ip}</code>
                    </TableCell>
                    <TableCell>
                      <code className="font-mono text-xs text-muted-foreground">{lease.mac}</code>
                    </TableCell>
                    {/*
                      Guest-supplied. React escapes it on render, and it stays a
                      text node — no link, no title attribute, no id.
                    */}
                    <TableCell className="text-muted-foreground">{lease.hostname || '—'}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {lease.guestKind ? `${lease.guestKind} · ${lease.guestId ?? ''}` : '—'}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{formatAbsolute(lease.expiresAt)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <p className="text-xs text-muted-foreground">{t('networks.leases.hostnameNote')}</p>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
