// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SetupGuide for snapshot-synology. Walks the operator through the
// DSM-side prerequisites: enabling the Backup Service API, creating
// an app-specific password, and selecting the share to snapshot.

import React from 'react';
import { useTranslation } from 'react-i18next';
import { Section, Field } from '@resources/components/ui/Form';
import { Alert } from '@resources/components/ui/Alert';

export interface SetupGuideProps {
  driver: string;
  locale?: string;
}

export function SetupGuide(_props: SetupGuideProps) {
  const { t } = useTranslation('snapshot-synology');
  const steps = t('setup.steps', { returnObjects: true }) as Array<{
    title: string;
    body: string;
    code?: string;
  }>;

  return (
    <div className="space-y-4">
      <Alert variant="info">{t('setup.intro')}</Alert>
      <Section title={t('setup.title')}>
        {steps.map((step) => (
          <Field key={step.title} label={step.title} description={step.body}>
            {step.code ? (
              <pre className="rounded border border-border bg-muted/30 px-3 py-2 font-mono text-xs whitespace-pre-wrap">
                {step.code}
              </pre>
            ) : null}
          </Field>
        ))}
      </Section>
    </div>
  );
}
