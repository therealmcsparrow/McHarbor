// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  CATEGORY_TAG_COLORS,
  getNodeDocumentation,
  getNodeRequirement,
  isNodeDefinitionAvailable,
} from '../nodes';
import { useNodeAvailability } from '@resources/hooks/useNodeAvailability';
import type { ConfigField, NodeDefinition } from '../types';
import {
  describeFieldType,
  describePort,
  formatDefaultValue,
  getBehaviorSummary,
  getRequirementMessage,
  parseMarkdownSections,
  type MarkdownSection,
  type NodeDocs,
} from './help-tab-utils';

export function HelpTabNodeDocumentation({ definition }: { definition: NodeDefinition }) {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const capabilities = useNodeAvailability();
  const requirement = getNodeRequirement(definition);
  const isAvailable = isNodeDefinitionAvailable(definition, capabilities);
  const [docs, setDocs] = useState<NodeDocs>();
  const [loadingDocs, setLoadingDocs] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoadingDocs(true);
    setDocs(undefined);

    void getNodeDocumentation(definition.key).then((result) => {
      if (cancelled) {
        return;
      }

      setDocs(result);
      setLoadingDocs(false);
    });

    return () => {
      cancelled = true;
    };
  }, [definition.key]);

  const readmeSections = parseMarkdownSections(docs?.readme);
  const technicalSections = parseMarkdownSections(docs?.backendAction);
  const tagColor = CATEGORY_TAG_COLORS[definition.category] ?? '';

  return (
    <div className="space-y-4 p-4">
      <div>
        <div className="flex items-center gap-2">
          <span className={cn('rounded-md px-2 py-0.5 text-[10px] font-medium uppercase', tagColor)}>
            {definition.category}
          </span>
          <span className="text-sm font-medium text-foreground">
            {tn(`${definition.key}.label`, { defaultValue: definition.label })}
          </span>
        </div>
        <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">
          {tn(`${definition.key}.description`, { defaultValue: definition.description })}
        </p>
      </div>

      {requirement && (
        <section
          className={cn(
            'rounded-lg border p-3',
            isAvailable ? 'border-emerald-500/20 bg-emerald-500/5' : 'border-amber-500/20 bg-amber-500/5',
          )}
        >
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs font-semibold text-foreground">
              {t('workflows.requirementsSection', { defaultValue: 'Requirements' })}
            </p>
            <Badge variant="secondary" className="text-[10px]">
              {isAvailable
                ? t('workflows.configured', { defaultValue: 'Configured' })
                : t('workflows.unavailable', { defaultValue: 'Unavailable' })}
            </Badge>
          </div>
          <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
            {getRequirementMessage(requirement.kind, t)}
          </p>
        </section>
      )}

      <HelpSection title={t('workflows.behaviorSection', { defaultValue: 'Behavior' })}>
        {getBehaviorSummary(definition.category, t).map((paragraph) => (
          <p key={paragraph} className="text-[11px] leading-relaxed text-muted-foreground">
            {paragraph}
          </p>
        ))}
      </HelpSection>

      <HelpSection title={t('workflows.inputPorts', { defaultValue: 'Input Ports' })}>
        {definition.inputPorts.length > 0 ? (
          <div className="space-y-2">
            {definition.inputPorts.map((port) => (
              <PortCard key={port} label={port} description={describePort(port, 'input', t)} />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">
            {t('workflows.noInputs', {
              defaultValue: 'This node starts a flow and does not receive incoming connections.',
            })}
          </p>
        )}
      </HelpSection>

      <HelpSection title={t('workflows.outputPorts', { defaultValue: 'Output Ports' })}>
        {definition.outputPorts.length > 0 ? (
          <div className="space-y-2">
            {definition.outputPorts.map((port) => (
              <PortCard key={port} label={port} description={describePort(port, 'output', t)} />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">
            {t('workflows.noOutputs', { defaultValue: 'This node does not expose downstream outputs.' })}
          </p>
        )}
      </HelpSection>

      <HelpSection title={t('workflows.configurationSection', { defaultValue: 'Configuration' })}>
        {definition.configSchema.length > 0 ? (
          <div className="space-y-2">
            {definition.configSchema.map((field) => (
              <FieldCard key={field.key} definition={definition} field={field} />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">
            {t('workflows.noConfig', { defaultValue: 'This node has no configurable fields.' })}
          </p>
        )}
      </HelpSection>

      {loadingDocs && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Spinner size="sm" />
          <span>{t('loading')}</span>
        </div>
      )}

      {readmeSections.map((section) => (
        <DocSection key={`readme-${section.title}`} section={section} />
      ))}

      {technicalSections.length > 0 && (
        <HelpSection title={t('workflows.technicalDetailsSection', { defaultValue: 'Technical Details' })}>
          <div className="space-y-3">
            {technicalSections.map((section) => (
              <DocSectionContent key={`technical-${section.title}`} section={section} />
            ))}
          </div>
        </HelpSection>
      )}
    </div>
  );
}

function FieldCard({ definition, field }: { definition: NodeDefinition; field: ConfigField }) {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const translatedLabel = tn(`${definition.key}.config.${field.key}`, { defaultValue: field.label });

  return (
    <div className="rounded-md border border-border p-2.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-xs font-medium text-foreground">{translatedLabel}</span>
        <Badge variant="secondary" className="text-[10px]">{field.type}</Badge>
        <span className="text-[10px] text-muted-foreground">
          {field.required ? t('workflows.required', { defaultValue: 'required' }) : t('labels.optional', { defaultValue: 'optional' })}
        </span>
      </div>
      <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
        {describeFieldType(field.type, t)}
      </p>
      {field.default != null && (
        <p className="mt-1 text-[10px] text-muted-foreground">
          {t('workflows.defaultValue', { defaultValue: 'Default' })}:{' '}
          <code className="rounded bg-muted px-1 py-0.5 text-[10px]">{formatDefaultValue(field.default)}</code>
        </p>
      )}
      {field.options && field.options.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {field.options.map((option) => (
            <code key={option.value} className="rounded bg-muted px-1 py-0.5 text-[10px]">
              {tn(`${definition.key}.options.${field.key}.${option.value}`, { defaultValue: option.label })}
            </code>
          ))}
        </div>
      )}
    </div>
  );
}

function PortCard({ label, description }: { label: string; description: string }) {
  return (
    <div className="rounded-md border border-border p-2.5">
      <div className="flex items-center gap-2">
        <Badge variant="secondary" className="text-[10px]">{label}</Badge>
      </div>
      <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">{description}</p>
    </div>
  );
}

function HelpSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h3 className="mb-2 text-xs font-semibold text-foreground">{title}</h3>
      <div className="space-y-2">{children}</div>
    </section>
  );
}

function DocSection({ section }: { section: MarkdownSection }) {
  return (
    <HelpSection title={section.title}>
      <DocSectionContent section={section} />
    </HelpSection>
  );
}

function DocSectionContent({ section }: { section: MarkdownSection }) {
  return (
    <>
      {section.paragraphs.map((paragraph) => (
        <p key={paragraph} className="text-[11px] leading-relaxed text-muted-foreground">
          {paragraph}
        </p>
      ))}
      {section.bullets.length > 0 && (
        <ul className="space-y-1">
          {section.bullets.map((bullet) => (
            <li key={bullet} className="text-[11px] leading-relaxed text-muted-foreground">
              - {bullet}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
