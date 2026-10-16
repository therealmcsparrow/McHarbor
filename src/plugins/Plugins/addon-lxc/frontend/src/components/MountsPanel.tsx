// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { CT_PERM } from '../permissions';
import { useDeleteMount, useDeleteNIC } from '../hooks/useContainers';
import { isLive, type Container, type Mount } from '../types';

/**
 * A container's mounts and interfaces.
 *
 * Mounts get more room than interfaces because they carry the plugin's one
 * genuine security decision: a volume is a directory inside the container's own
 * pool, and a bind is any path on the host. The second is a host-compromise
 * vector — bind the host's /etc into a container and a root process inside it
 * can rewrite the host's passwords — so the two are visually distinct, the bind
 * rows carry a warning, and removing one takes the privileged permission.
 *
 * Changes need a stopped container. That is a uniform rule rather than a
 * per-mount one: some mount changes could be applied live, and offering them
 * selectively would mean an operator learning which by trying them on something
 * that matters.
 */
export function MountsPanel({ container }: { container: Container }) {
  const { t } = useTranslation('ct');
  const deleteMount = useDeleteMount();
  const deleteNIC = useDeleteNIC();

  const mutable = container.hostAvailable && !isLive(container);
  const mounts = container.mounts ?? [];
  const nics = container.nics ?? [];
  const hasHostMounts = mounts.some((mount) => mount.kind !== 'volume');

  return (
    <div className="space-y-6">
      {!mutable && container.hostAvailable && (
        <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">{t('mounts.mustBeStopped')}</p>
      )}

      <section>
        <h3 className="mb-2 font-medium text-foreground">{t('mounts.title')}</h3>

        {/*
          Shown once above the table rather than on every row: repeating it per
          mount would make it wallpaper, and the point is for it to still
          register on the tenth container somebody looks at.
        */}
        {hasHostMounts && (
          <div className="mb-2 flex items-start gap-2 rounded-md bg-warning/10 p-3">
            <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <p className="text-sm text-foreground">{t('mounts.hostBindWarning')}</p>
          </div>
        )}

        {mounts.length === 0 ? (
          <p className="rounded-lg border border-border p-4 text-sm text-muted-foreground">{t('mounts.empty')}</p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('mounts.columns.kind')}</TableHead>
                  <TableHead>{t('mounts.columns.source')}</TableHead>
                  <TableHead>{t('mounts.columns.destination')}</TableHead>
                  <TableHead>{t('mounts.columns.options')}</TableHead>
                  <TableHead className="w-16" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {mounts.map((mount) => (
                  <TableRow key={mount.id}>
                    <TableCell>
                      <MountKindBadge mount={mount} />
                    </TableCell>
                    <TableCell>
                      <code className="font-mono text-xs text-muted-foreground">{mount.source}</code>
                    </TableCell>
                    <TableCell>
                      <code className="font-mono text-xs text-foreground">{mount.destination}</code>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {mount.readonly && <Badge variant="outline">{t('mounts.readonly')}</Badge>}
                        {/*
                          Called out when it is *off*, not when it is on. Ids
                          shifted is the normal case; an unshifted mount in an
                          unprivileged container appears to belong to nobody
                          from the inside, and that is the single most common
                          "why can't my container write to this" question.
                        */}
                        {!mount.shiftIds && !container.privileged && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Badge variant="warning">{t('mounts.shiftIds')}</Badge>
                            </TooltipTrigger>
                            <TooltipContent>{t('mounts.shiftIdsHint')}</TooltipContent>
                          </Tooltip>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t('actions.delete')}
                        // A volume takes manage; a host bind or a device takes
                        // the privileged key, because removing one is the same
                        // class of change as adding it.
                        permission={mount.kind === 'volume' ? CT_PERM.MANAGE : CT_PERM.PRIVILEGED}
                        disabled={!mutable}
                        onClick={() => deleteMount.mutate({ id: container.id, mountId: mount.id })}
                      >
                        <IconTrash className="h-4 w-4" aria-hidden="true" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </section>

      <section>
        <h3 className="mb-2 font-medium text-foreground">{t('list.columns.address')}</h3>
        <div className="overflow-x-auto rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('list.columns.name')}</TableHead>
                <TableHead>{t('list.columns.host')}</TableHead>
                <TableHead>{t('list.columns.address')}</TableHead>
                <TableHead className="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {nics.map((nic) => (
                <TableRow key={nic.id}>
                  <TableCell>
                    <code className="font-mono text-xs text-foreground">{nic.guestName}</code>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {nic.bridge}
                    {nic.vlanTag ? ` · VLAN ${nic.vlanTag}` : ''}
                    {/*
                      The host-side name, because finding one container's veth
                      in `ip link` is otherwise a process of elimination.
                    */}
                    {nic.vethHost && <span className="ml-2 font-mono text-xs">{nic.vethHost}</span>}
                  </TableCell>
                  <TableCell>
                    <span className="font-mono text-xs text-muted-foreground">
                      {nic.addressMode === 'static' ? nic.staticIp : nic.addressMode}
                    </span>
                    {nic.staticIpv6 && (
                      <span className="block font-mono text-xs text-muted-foreground">{nic.staticIpv6}</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={t('actions.delete')}
                      permission={CT_PERM.MANAGE}
                      disabled={!mutable}
                      onClick={() => deleteNIC.mutate({ id: container.id, nicId: nic.id })}
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
    </div>
  );
}

function MountKindBadge({ mount }: { mount: Mount }) {
  const { t } = useTranslation('ct');
  const label = t(`mounts.kind.${mount.kind}`, mount.kind);
  const hint = t(`mounts.kindHint.${mount.kind}`, '');

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant={mount.kind === 'volume' ? 'secondary' : 'warning'}>{label}</Badge>
      </TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  );
}
