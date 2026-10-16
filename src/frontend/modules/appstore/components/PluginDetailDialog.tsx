// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload, IconExternalLink } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
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
import { Box, Flex } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import type { PluginEntry } from '../types/plugins';
import { PluginIcon } from './plugin-icons';
import { parseAuthor, PluginSignatureIcon } from './PluginSignature';
import { versionLabel } from '../utils/versionLabel';

interface PluginDetailDialogProps {
  plugin: PluginEntry | null;
  open: boolean;
  installed: boolean;
  onOpenChange: (open: boolean) => void;
  onInstall: (plugin: PluginEntry) => void;
}

export function PluginDetailDialog({
  plugin,
  open: isOpen,
  installed,
  onOpenChange,
  onInstall,
}: PluginDetailDialogProps) {
  const { t } = useTranslation(['common', 'plugins']);
  if (!plugin) return null;

  // Same treatment as the card: the name shows, the address only ever
  // lives in the href.
  const author = parseAuthor(plugin.author ?? '');

  const homepage = /^https?:\/\//i.test(plugin.homepage) ? plugin.homepage : null;

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg p-0">
        <DialogHeader>
          <Flex align="start" gap={3}>
            <Box className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <PluginIcon
                icon={plugin.icon}
                fromCatalog={Boolean(plugin.icon_url)}
                slug={plugin.slug}
                alt=""
                className="size-6"
              />
            </Box>
            <Box className="flex-1">
              <DialogTitle>{plugin.name}</DialogTitle>
              <DialogDescription className="sr-only">{plugin.description}</DialogDescription>
            </Box>
          </Flex>
        </DialogHeader>

        <DialogBody className="space-y-4 p-4">
          <TypographyText tone="muted">{plugin.description}</TypographyText>
          <Flex wrap gap={2}>
            {plugin.category && <Badge variant="secondary">{plugin.category}</Badge>}
            <Badge variant="outline">{versionLabel(plugin.version)}</Badge>
            {author.name && (
              <Badge variant="outline">
                {author.email ? (
                  <a
                    href={`mailto:${author.email}`}
                    className="rounded-sm text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                  >
                    {author.name}
                  </a>
                ) : (
                  author.name
                )}
              </Badge>
            )}
            {plugin.license && <Badge variant="outline">{plugin.license}</Badge>}
            <PluginSignatureIcon signed={!!plugin.signature} />
          </Flex>
          {homepage && (
            <a
              href={homepage}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 rounded-sm text-sm text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            >
              {t('appStore.website')}
              <IconExternalLink className="size-3.5" />
            </a>
          )}
        </DialogBody>

        <DialogFooter>
          {installed ? (
            <Badge variant="success">{t('catalog.installed', { ns: 'plugins' })}</Badge>
          ) : (
            <Button
              size="sm"
              className="focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => {
                onInstall(plugin);
                onOpenChange(false);
              }}
            >
              <IconDownload className="size-3.5" />
              {t('appStore.install')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
