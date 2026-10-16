// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { CATEGORY_TAG_COLORS, getNodeDocumentation, getNodeRequirement, isNodeDefinitionAvailable } from '../nodes';
import { useNodeAvailability } from '@resources/hooks/useNodeAvailability';
import type { NodeDefinition } from '../types';
import {
  describePort,
  getBehaviorSummary,
  getRequirementMessage,
  parseMarkdownSections,
  type NodeDocs,
} from './help-tab-utils';
import { DocSection, DocSectionContent, FieldCard, HelpSection, PortCard } from './HelpTabNodeDocumentationParts';

export function HelpTabNodeDocumentation({ definition }: { definition: NodeDefinition }) {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const capabilities = useNodeAvailability();
  const requirement = getNodeRequirement(definition);
  const isAvailable = isNodeDefinitionAvailable(definition, capabilities);
  const [loaded, setLoaded] = useState<{ key: string; docs: NodeDocs | undefined } | null>(null);
  const loadingDocs = loaded?.key !== definition.key;
  const docs = loadingDocs ? undefined : loaded?.docs;

  useEffect(() => {
    let cancelled = false;

    void getNodeDocumentation(definition.key).then((result) => {
      if (cancelled) {
        return;
      }

      setLoaded({ key: definition.key, docs: result });
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
            {/* i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback */}
            {tn(`${definition.key}.label`, { defaultValue: definition.label })}
          </span>
        </div>
        <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">
          {/* i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback */}
          {tn(`${definition.key}.description`, { defaultValue: definition.description })}
        </p>
      </div>

      {requirement && (
        <section
          className={cn(
            'rounded-lg border p-3',
            isAvailable ? 'border-success/20 bg-success/5' : 'border-warning/20 bg-warning/5',
          )}
        >
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs font-semibold text-foreground">{t('workflows.requirementsSection')}</p>
            <Badge variant="secondary" className="text-[10px]">
              {isAvailable ? t('workflows.configured') : t('workflows.unavailable')}
            </Badge>
          </div>
          <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
            {getRequirementMessage(requirement.kind, t)}
          </p>
        </section>
      )}

      <HelpSection title={t('workflows.behaviorSection')}>
        {getBehaviorSummary(definition.category, t).map((paragraph) => (
          <p key={paragraph} className="text-[11px] leading-relaxed text-muted-foreground">
            {paragraph}
          </p>
        ))}
      </HelpSection>

      <HelpSection title={t('workflows.inputPorts')}>
        {definition.inputPorts.length > 0 ? (
          <div className="space-y-2">
            {definition.inputPorts.map((port) => (
              <PortCard key={port} label={port} description={describePort(port, 'input', t)} />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">{t('workflows.noInputs')}</p>
        )}
      </HelpSection>

      <HelpSection title={t('workflows.outputPorts')}>
        {definition.outputPorts.length > 0 ? (
          <div className="space-y-2">
            {definition.outputPorts.map((port) => (
              <PortCard key={port} label={port} description={describePort(port, 'output', t)} />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">{t('workflows.noOutputs')}</p>
        )}
      </HelpSection>

      <HelpSection title={t('workflows.configurationSection')}>
        {definition.configSchema.length > 0 ? (
          <div className="space-y-2">
            {definition.configSchema.map((field) => (
              <FieldCard key={field.key} definition={definition} field={field} />
            ))}
          </div>
        ) : (
          <p className="text-[11px] leading-relaxed text-muted-foreground">{t('workflows.noConfig')}</p>
        )}
      </HelpSection>

      {loadingDocs && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Spinner size="sm" />
          <span>{t('common.loading')}</span>
        </div>
      )}

      {readmeSections.map((section) => (
        <DocSection key={`readme-${section.title}`} section={section} />
      ))}

      {technicalSections.length > 0 && (
        <HelpSection title={t('workflows.technicalDetailsSection')}>
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
