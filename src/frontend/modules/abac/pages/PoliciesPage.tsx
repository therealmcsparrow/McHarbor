// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PageHeader } from '@resources/layout/PageHeader';
import { PoliciesSection } from '@resources/components/security/PoliciesSection';

export default function PoliciesPage() {
  const { t } = useTranslation('abac');
  return (
    <div className="space-y-6">
      <PageHeader title={t('title')} description={t('description')} headingLeaf />
      <PoliciesSection />
    </div>
  );
}
