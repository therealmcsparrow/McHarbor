// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The whole activation flow: paste the key McHarbor issued, claim a seat.
//
// There is no form to request a licence from in here any more — keys are
// issued at mcharbor.eu, and this dialog only hands one to the licence
// server. The error copy matters more than the input does: an NFR key
// arrives dormant, so "waiting for approval" and "invalid key" are the
// same HTTP status and very different messages to read.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconExternalLink } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { FormField } from '@resources/components/ui/FormField';
import { Input } from '@resources/components/ui/Input';
import { Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { appToast } from '@resources/utils/toast';
import { PERM } from '@core/auth/permissionKeys';
import { useActivateLicense } from '../hooks/useLicense';

// Requests are made on the website, not in here — see the note above.
const licenseRequestUrl = 'https://www.mcharbor.eu/licensing/request';

type ActivateLicenseDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Pre-fills nothing, but changes the wording when one is already active. */
  hasLicense: boolean;
};

export function ActivateLicenseDialog({ open: isOpen, onOpenChange, hasLicense }: ActivateLicenseDialogProps) {
  const { t } = useTranslation('license');
  const activate = useActivateLicense();

  const [key, setKey] = useState('');

  const handleSubmit = () => {
    activate.mutate(
      { key: key.trim() },
      {
        onSuccess: () => {
          appToast.success(t('activate.success'));
          setKey('');
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{hasLicense ? t('activate.change') : t('activate.title')}</DialogTitle>
          <DialogDescription>{t('activate.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody>
          <Stack gap={4}>
            <FormField label={t('activate.key')} htmlFor="license-key" description={t('activate.keyHint')}>
              <Input
                id="license-key"
                value={key}
                placeholder={t('activate.keyPlaceholder')}
                autoComplete="off"
                spellCheck={false}
                onChange={(e) => setKey(e.target.value)}
              />
            </FormField>

            <TypographyText tone="muted" size="xs">
              {t('activate.seatNote')}
            </TypographyText>
          </Stack>
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" asChild>
            <a href={licenseRequestUrl} target="_blank" rel="noreferrer">
              <IconExternalLink className="size-4" /> {t('activate.request')}
            </a>
          </Button>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('activate.cancel')}
          </Button>
          <Button
            permission={PERM.LICENSE_MANAGE}
            disabled={key.trim().length === 0 || activate.isPending}
            onClick={handleSubmit}
          >
            {activate.isPending ? t('activate.submitting') : t('activate.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
