// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { CreateVMForm } from '../../hooks/useCreateVMForm';
import { TemplateSourceFields } from '../CreateFromTemplate';
import type { Source } from './model';

const SOURCES: Source[] = ['image', 'iso', 'blank', 'template'];

export function CreateSourceStep({
  form,
  environments,
}: {
  form: CreateVMForm;
  environments: { id: string; name: string }[];
}) {
  const { t } = useTranslation('vm');
  const {
    environmentId,
    setEnvironmentId,
    nameValue,
    setName,
    source,
    setSource,
    imageId,
    setImageId,
    isoId,
    setIsoId,
    setPoolId,
    setDiskMode,
    setVolumePath,
    setNetworkId,
    templateId,
    setTemplateId,
    linked,
    setLinked,
    hostTemplates,
    readyImages,
    readyDiscs,
    nameErrorKey,
  } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.source.title')}</h3>

      {environments.length > 1 && (
        <div>
          <Label>{t('list.columns.host')}</Label>
          <Select
            value={environmentId}
            ariaLabel={t('list.columns.host')}
            options={environments.map((env) => ({
              value: env.id,
              label: env.name,
            }))}
            onChange={(value) => {
              setEnvironmentId(value);
              // Pools, networks, images and volumes are all per host.
              setPoolId('');
              setNetworkId('');
              setImageId('');
              setVolumePath('');
            }}
          />
        </div>
      )}

      <div>
        <Label htmlFor="vm-name">{t('list.columns.name')}</Label>
        <Input
          id="vm-name"
          value={nameValue}
          onChange={(evt) => setName(evt.target.value)}
          autoComplete="off"
          aria-invalid={nameErrorKey !== null}
          aria-describedby={nameErrorKey ? 'vm-name-error' : undefined}
        />
        {nameErrorKey && (
          <p id="vm-name-error" className="mt-1 text-xs text-destructive">
            {t(nameErrorKey)}
          </p>
        )}
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        {SOURCES.map((entry) => (
          <Button
            key={entry}
            variant="outline"
            aria-pressed={source === entry}
            onClick={() => {
              setSource(entry);
              if (entry === 'image') setDiskMode('new');
            }}
            className={cn(
              'h-auto flex-col items-start gap-1 p-3 text-left whitespace-normal',
              source === entry && 'border-primary bg-primary/5',
            )}
          >
            <span className="text-sm font-medium text-foreground">{t(`create.source.${entry}`)}</span>
            <span className="text-xs font-normal text-muted-foreground">{t(`create.source.${entry}Hint`)}</span>
          </Button>
        ))}
      </div>

      {source === 'template' ? (
        <TemplateSourceFields
          templates={hostTemplates}
          templateId={templateId}
          linked={linked}
          onTemplateChange={setTemplateId}
          onLinkedChange={setLinked}
        />
      ) : source === 'blank' ? (
        <p className="text-sm text-muted-foreground">{t('create.source.blankNote')}</p>
      ) : source === 'image' ? (
        <div>
          <Label>{t('create.source.image')}</Label>
          {readyImages.length === 0 ? (
            <p className="text-sm text-warning">{t('create.source.noImages')}</p>
          ) : (
            <Select
              value={imageId}
              ariaLabel={t('create.source.image')}
              options={readyImages.map((entry) => ({
                value: entry.id,
                label: `${entry.displayName} (${entry.arch})`,
              }))}
              onChange={setImageId}
            />
          )}
        </div>
      ) : (
        <div>
          <Label>{t('create.source.isoLabel')}</Label>
          {readyDiscs.length === 0 ? (
            <p className="text-sm text-warning">{t('create.source.noIsos')}</p>
          ) : (
            <Select
              value={isoId}
              ariaLabel={t('create.source.isoLabel')}
              options={readyDiscs.map((entry) => ({
                value: entry.id,
                label: `${entry.displayName} (${entry.arch})`,
              }))}
              onChange={setIsoId}
            />
          )}
        </div>
      )}
    </>
  );
}
