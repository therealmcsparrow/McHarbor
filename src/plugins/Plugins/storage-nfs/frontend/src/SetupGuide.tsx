// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SetupGuide walks the operator through preparing an NFS
// destination for use with the storage-nfs plugin. It's
// rendered when the host's plugin slot is in 'setup_guide'
// mode.
//
// Sections cover the four sec flavors. The AUTH_SYS path is
// the simplest and is what most home/small-business users
// need; the Kerberos paths require keytab provisioning on
// the NFS server side which is the operator's
// responsibility.
//
// The plugin does not call react-i18next's useTranslation (the
// bundle has no i18next instance); strings come from the
// host-supplied `t`, in the namespace named after the plugin slug.

import { IconServer, IconLock, IconKey, IconFolder } from '@tabler/icons-react';

const KEYS = {
  title: 'storage-nfs.guide.title',
  p1: 'storage-nfs.guide.description',
  p2: 'storage-nfs.guide.section.server.body',
  p3: 'storage-nfs.form.guide.exportBody',
  p4: 'storage-nfs.guide.section.sec.body',
  p5: 'storage-nfs.guide.section.sec.body_sys',
  p6: 'storage-nfs.form.guide.krb5Body',
  p7: 'storage-nfs.form.guide.krb5iBody',
  p8: 'storage-nfs.guide.section.sec.body_krb5p',
  p9: 'storage-nfs.guide.section.keytab.body',
  p10: 'storage-nfs.form.guide.testBody',
  s11: 'storage-nfs.guide.section.server.title',
  s12: 'storage-nfs.guide.section.export.title',
  s13: 'storage-nfs.guide.section.sec.title',
  s14: 'storage-nfs.guide.section.keytab.title',
  s15: 'storage-nfs.guide.section.capabilities.title',
};

export interface SetupGuideProps {
  value?: Record<string, string>;
  t?: (key: string, vars?: Record<string, unknown>) => string;
}

export function SetupGuide({ value, t: hostT }: SetupGuideProps) {
  const t = (key: string, vars?: Record<string, unknown>) => hostT?.(key, vars) ?? key;

  const sec = value?.sec ?? 'sys';

  return (
    <div className="space-y-6">
      <header>
        <h3 className="text-base font-semibold text-foreground">{t(KEYS.title)}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{t(KEYS.p1)}</p>
      </header>

      <Section icon={<IconServer className="h-4 w-4" />} title={t(KEYS.s11)}>
        <p className="text-sm text-muted-foreground">{t(KEYS.p2)}</p>
      </Section>

      <Section icon={<IconFolder className="h-4 w-4" />} title={t(KEYS.s12)}>
        <p className="text-sm text-muted-foreground">{t(KEYS.p3)}</p>
      </Section>

      <Section icon={<IconLock className="h-4 w-4" />} title={t(KEYS.s13)}>
        <p className="text-sm text-muted-foreground">{t(KEYS.p4)}</p>
        {sec === 'sys' && <p className="mt-2 text-sm text-foreground">{t(KEYS.p5)}</p>}
        {sec === 'krb5' && <p className="mt-2 text-sm text-foreground">{t(KEYS.p6)}</p>}
        {sec === 'krb5i' && <p className="mt-2 text-sm text-foreground">{t(KEYS.p7)}</p>}
        {sec === 'krb5p' && <p className="mt-2 text-sm text-foreground">{t(KEYS.p8)}</p>}
      </Section>

      {sec !== 'sys' && (
        <Section icon={<IconKey className="h-4 w-4" />} title={t(KEYS.s14)}>
          <p className="text-sm text-muted-foreground">{t(KEYS.p9)}</p>
        </Section>
      )}

      <Section icon={<IconServer className="h-4 w-4" />} title={t(KEYS.s15)}>
        <p className="text-sm text-muted-foreground">{t(KEYS.p10)}</p>
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

export default SetupGuide;
