// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SetupGuide walks the operator through preparing an FTP/FTPS
// destination for use with the storage-ftp plugin. It's
// rendered when the host's plugin slot is in 'guide' mode.
//
// Sections cover the five auth methods. The body section
// branches on cfg.auth_method so the operator sees only the
// steps that apply to their setup.

import { useTranslation } from 'react-i18next';
import { IconServer, IconLock, IconKey, IconCertificate, IconFolder, IconShieldCheck } from '@tabler/icons-react';

export interface SetupGuideProps {
  cfg?: Record<string, string>;
}

export default function SetupGuide({ cfg }: SetupGuideProps) {
  const { t } = useTranslation('storage-ftp');

  const authMethod = cfg?.auth_method ?? 'password';

  return (
    <div className="space-y-6">
      <header>
        <h3 className="text-base font-semibold text-foreground">{t('guide.title')}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{t('guide.description')}</p>
      </header>

      <Section icon={<IconServer className="h-4 w-4" />} title={t('guide.section.server.title')}>
        <p className="text-sm text-muted-foreground">{t('guide.section.server.body')}</p>
      </Section>

      <Section icon={<IconLock className="h-4 w-4" />} title={t('guide.section.auth.title')}>
        <p className="text-sm text-muted-foreground">{t('guide.section.auth.body')}</p>
        {authMethod === 'anonymous' && (
          <p className="mt-2 text-sm text-foreground">{t('guide.section.auth.body_anonymous')}</p>
        )}
        {authMethod === 'password' && (
          <p className="mt-2 text-sm text-foreground">{t('guide.section.auth.body_password')}</p>
        )}
        {authMethod === 'ftps_explicit' && (
          <p className="mt-2 text-sm text-foreground">{t('guide.section.auth.body_ftps_explicit')}</p>
        )}
        {authMethod === 'ftps_implicit' && (
          <p className="mt-2 text-sm text-foreground">{t('guide.section.auth.body_ftps_implicit')}</p>
        )}
        {authMethod === 'client_cert' && (
          <p className="mt-2 text-sm text-foreground">{t('guide.section.auth.body_client_cert')}</p>
        )}
      </Section>

      {(authMethod === 'password' || authMethod === 'ftps_explicit' || authMethod === 'ftps_implicit') && (
        <Section icon={<IconKey className="h-4 w-4" />} title={t('guide.section.credentials.title')}>
          <p className="text-sm text-muted-foreground">{t('guide.section.credentials.body')}</p>
        </Section>
      )}

      {authMethod === 'client_cert' && (
        <Section icon={<IconCertificate className="h-4 w-4" />} title={t('guide.section.cert.title')}>
          <p className="text-sm text-muted-foreground">{t('guide.section.cert.body')}</p>
        </Section>
      )}

      {(authMethod === 'ftps_explicit' || authMethod === 'ftps_implicit' || authMethod === 'client_cert') && (
        <Section icon={<IconShieldCheck className="h-4 w-4" />} title={t('guide.section.tls.title')}>
          <p className="text-sm text-muted-foreground">{t('guide.section.tls.body')}</p>
        </Section>
      )}

      <Section icon={<IconFolder className="h-4 w-4" />} title={t('guide.section.path.title')}>
        <p className="text-sm text-muted-foreground">{t('guide.section.path.body')}</p>
      </Section>

      <Section icon={<IconServer className="h-4 w-4" />} title={t('guide.section.test.title')}>
        <p className="text-sm text-muted-foreground">{t('guide.section.test.body')}</p>
      </Section>
    </div>
  );
}

function Section({ icon, title, children }: { icon: React.ReactNode; title: string; children: React.ReactNode }) {
  return (
    <section className="rounded-lg border border-border bg-card p-4">
      <h4 className="mb-2 flex items-center gap-2 text-sm font-semibold text-foreground">
        <span className="text-muted-foreground">{icon}</span>
        {title}
      </h4>
      <div className="text-sm">{children}</div>
    </section>
  );
}
