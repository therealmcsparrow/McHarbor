// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useLayoutEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { IconHome } from '@tabler/icons-react';
import { Card } from '@resources/components/ui/Card';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import type { SubNavItem } from '@resources/components/SubNav';
import { useExport, useReport } from '../hooks/useReporting';
import { useSectionSpy } from '../hooks/useSectionSpy';
import { DocumentView, sectionAnchorId } from '../blocks/DocumentView';
import { ViewerHeader } from '../blocks/ViewerHeader';
import { ViewerHero } from '../blocks/ViewerHero';

const PANEL_ID = 'report-document-panel';

const SHELL = 'flex h-full min-h-0 w-full flex-col gap-4';

type Props = {
  id: string;
  onBack: () => void;
};

export function ViewerPage({ id, onBack }: Props) {
  const { t } = useTranslation('reporting');
  const report = useReport(id);
  const download = useExport();
  const scrollRef = useRef<HTMLDivElement>(null);

  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  useLayoutEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  const doc = report.data?.document;

  const sections: SubNavItem<string>[] = useMemo(
    () =>
      (doc?.sections ?? []).map((section, i) => ({
        id: sectionAnchorId(section.level, i),
        label: section.title,
      })),
    [doc],
  );
  const sectionIds = useMemo(() => sections.map((s) => s.id), [sections]);
  const activeSection = useSectionSpy(scrollRef, sectionIds);

  const goToSection = useCallback((anchor: string) => {
    const el = scrollRef.current?.querySelector(`#${CSS.escape(anchor)}`);
    el?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }, []);

  const exportAs = useCallback(
    (format: 'pdf' | 'docx') => {
      if (!doc) return;
      download.mutate({ id, format, name: doc.meta.title });
    },
    [doc, download, id],
  );

  if (report.isLoading) {
    return (
      <div className={SHELL}>
        <CardSkeleton lines={8} />
      </div>
    );
  }
  if (report.isError || !doc) {
    return (
      <div className={SHELL}>
        <ErrorState
          description={t('viewer.error')}
          onRetry={() => {
            void report.refetch();
          }}
        />
      </div>
    );
  }

  return (
    <div className={SHELL}>
      <HeaderBreadcrumb
        crumbs={[
          { to: '/dashboard', label: <IconHome className="size-3" /> },
          { label: t('page.library.title'), onClick: onBack },
          { label: doc.meta.title },
        ]}
      />

      <ViewerHeader meta={doc.meta} scorecard={doc.scorecard} onExport={exportAs} exporting={download.isPending} />

      {sections.length > 0 && (
        <Card className="flex-none overflow-visible">
          <ViewerHero
            sections={sections}
            activeSection={activeSection}
            onSelectSection={goToSection}
            panelId={PANEL_ID}
          />
        </Card>
      )}

      <div ref={scrollRef} id={PANEL_ID} className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
        <DocumentView doc={doc} />
      </div>
    </div>
  );
}
