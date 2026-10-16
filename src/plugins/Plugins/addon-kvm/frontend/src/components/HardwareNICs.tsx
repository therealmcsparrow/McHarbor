// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { VM_PERM } from '../permissions';
import { useDeleteNIC, useUpdateNIC } from '../hooks/useVMs';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { VM } from '../types';

export function HardwareNICs({
  vm,
  mutable,
  live,
  onAdd,
}: {
  vm: VM;
  mutable: boolean;
  /** Running with hot-plug: interfaces come, go and change their link live. */
  live: boolean;
  onAdd: () => void;
}) {
  const { t } = useTranslation('vm');
  const deleteNIC = useDeleteNIC();
  const updateNIC = useUpdateNIC();
  const manage = usePermissionGate(VM_PERM.MANAGE);
  const changeable = mutable || live;
  const nics = vm.nics ?? [];

  return (
    <section>
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="font-medium text-foreground">{t('hardware.nics')}</h3>
        <Button variant="outline" size="sm" permission={VM_PERM.MANAGE} disabled={!changeable} onClick={onAdd}>
          <IconPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('actions.addNic')}
        </Button>
      </div>
      <div className="overflow-x-auto rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('hardware.nicColumns.slot')}</TableHead>
              <TableHead>{t('hardware.nicColumns.model')}</TableHead>
              <TableHead>{t('hardware.nicColumns.mac')}</TableHead>
              <TableHead>{t('hardware.nicColumns.bridge')}</TableHead>
              <TableHead>{t('hardware.nicColumns.link')}</TableHead>
              <TableHead className="w-16" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {nics.map((nic) => (
              <TableRow key={nic.id}>
                <TableCell>{nic.slot}</TableCell>
                <TableCell className="text-muted-foreground">{nic.model}</TableCell>
                <TableCell>
                  <code className="font-mono text-xs text-muted-foreground">{nic.mac}</code>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {nic.bridge}
                  {nic.vlanTag ? ` · VLAN ${nic.vlanTag}` : ''}
                </TableCell>
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span>
                        <Switch
                          checked={!nic.linkDown}
                          aria-label={t('hardware.linkToggle')}
                          disabled={!changeable || !manage.allowed || updateNIC.isPending}
                          onCheckedChange={(linkUp) => updateNIC.mutate({ id: vm.id, nicId: nic.id, linkUp })}
                        />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>{manage.denied ? manage.reason : t('hardware.linkToggle')}</TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('actions.delete')}
                    permission={VM_PERM.MANAGE}
                    disabled={!changeable}
                    onClick={() => deleteNIC.mutate({ id: vm.id, nicId: nic.id })}
                  >
                    <IconTrash className="h-4 w-4" aria-hidden="true" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}
