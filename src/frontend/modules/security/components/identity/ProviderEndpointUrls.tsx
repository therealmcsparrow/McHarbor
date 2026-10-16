// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import type { IdentityAdapterSpec } from '@resources/hooks/useIdentityAdapters';
import { useProviderEndpointUrls } from '../../hooks/useIdentitySetup';
import { CopyField } from './CopyField';

type Props = {
  providerId?: string;
  capabilities?: IdentityAdapterSpec['Capabilities'];
};

export function ProviderEndpointUrls({ providerId, capabilities }: Props) {
  const { t } = useTranslation('access');
  const { data, isLoading } = useProviderEndpointUrls(providerId);
  if (isLoading) return <Spinner />;
  if (!data) return null;
  const saml = Boolean(capabilities?.SAMLRedirect || capabilities?.SupportsMetadata);
  const oidc = capabilities ? capabilities.OIDCRedirect : true;
  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <p className="text-xs text-muted-foreground">{t('endpoints.intro')}</p>
      {oidc && (
        <CopyField
          label={t('endpoints.oidcRedirect')}
          value={data.oidcRedirectUri}
          hint={t('endpoints.oidcRedirectHint')}
        />
      )}
      {saml && data.samlAcsUrl && <CopyField label={t('endpoints.acs')} value={data.samlAcsUrl} />}
      {saml && data.samlMetadataUrl && <CopyField label={t('endpoints.spMetadata')} value={data.samlMetadataUrl} />}
      {saml && !providerId && <p className="text-xs text-muted-foreground">{t('endpoints.samlAfterSave')}</p>}
      {data.loginUrl && <CopyField label={t('endpoints.login')} value={data.loginUrl} />}
    </div>
  );
}
