// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileImport } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import { PERM } from '@core/auth/permissionKeys';
import { applicablePrefill, useMetadataImport, type MetadataImportResult } from '../../hooks/useIdentitySetup';
import { CopyField } from './CopyField';

type Props = {
  schema?: unknown[];
  onPrefill: (values: Record<string, string>) => void;
};

function ParsedMetadata({ result }: { result: MetadataImportResult }) {
  const { t } = useTranslation('access');
  const md = result.metadata;
  return (
    <div className="space-y-3">
      {md.entityId && <CopyField label={t('metadata.entityId')} value={md.entityId} />}
      {md.ssoServices.map((sso) => (
        <CopyField
          key={`${sso.binding}-${sso.location}`}
          label={t('metadata.sso', { binding: sso.binding.split(':').pop() })}
          value={sso.location}
        />
      ))}
      {md.sloServices.map((slo) => (
        <CopyField
          key={`slo-${slo.binding}-${slo.location}`}
          label={t('metadata.slo', { binding: slo.binding.split(':').pop() })}
          value={slo.location}
        />
      ))}
      {md.certificates.map((cert) => (
        <CopyField
          key={`${cert.use ?? ''}-${cert.fingerprintSha256}`}
          label={t('metadata.certificate', { use: cert.use || t('metadata.anyUse') })}
          value={cert.fingerprintSha256}
          hint={[cert.subject, cert.notAfter ? t('metadata.expires', { date: cert.notAfter }) : '']
            .filter(Boolean)
            .join(' · ')}
        />
      ))}
      {md.nameIdFormats.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t('metadata.nameIdFormats', { formats: md.nameIdFormats.join(', ') })}
        </p>
      )}
    </div>
  );
}

export function MetadataImportPanel({ schema, onPrefill }: Props) {
  const { t } = useTranslation('access');
  const [url, setUrl] = useState('');
  const [xml, setXml] = useState('');
  const importer = useMetadataImport();
  const result = importer.data;
  const prefill = result ? applicablePrefill(result.prefill, schema) : {};
  const prefillCount = Object.keys(prefill).length;

  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <div>
        <p className="text-sm font-medium text-foreground">{t('metadata.title')}</p>
        <p className="text-xs text-muted-foreground">{t('metadata.description')}</p>
      </div>
      <div>
        <Label htmlFor="metadata-url" className="mb-1">
          {t('metadata.url')}
        </Label>
        <Input
          variant="outline"
          id="metadata-url"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder={t('metadata.urlPlaceholder')}
        />
      </div>
      <div>
        <Label htmlFor="metadata-xml" className="mb-1">
          {t('metadata.xml')}
        </Label>
        <Textarea
          id="metadata-xml"
          rows={4}
          value={xml}
          onChange={(e) => setXml(e.target.value)}
          placeholder={t('metadata.xmlPlaceholder')}
        />
      </div>
      <Button
        variant="outline"
        size="sm"
        permission={PERM.IDENTITY_PROVIDERS_MANAGE}
        disabled={importer.isPending || (url.trim() === '' && xml.trim() === '')}
        onClick={() => importer.mutate(url.trim() ? { url: url.trim() } : { xml })}
      >
        <IconFileImport className="size-4" />
        {t('metadata.import')}
      </Button>
      {importer.isError && <Alert variant="destructive">{importer.error.message}</Alert>}
      {result && (
        <>
          {prefillCount > 0 ? (
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-muted/40 p-2">
              <span className="text-xs text-foreground">{t('metadata.prefillReady', { count: prefillCount })}</span>
              <Button size="sm" onClick={() => onPrefill(prefill)}>
                {t('metadata.applyPrefill')}
              </Button>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">{t('metadata.copyManually')}</p>
          )}
          <ParsedMetadata result={result} />
        </>
      )}
    </div>
  );
}
