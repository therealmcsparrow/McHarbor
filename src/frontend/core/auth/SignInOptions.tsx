// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAddressBook, IconBrandAzure, IconBrandGoogle, IconShieldLock, IconUser } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import type { OIDCProvider } from './useAuth';

const PROVIDER_ICONS: Record<string, typeof IconBrandAzure> = {
  entra_id: IconBrandAzure,
  google: IconBrandGoogle,
  generic_oidc: IconShieldLock,
  saml_2_0: IconShieldLock,
};

function directoryProviders(providers: OIDCProvider[]) {
  return providers.filter((provider) => provider.providerType === 'ldap');
}

export function DirectoryPicker({
  providers,
  value,
  onChange,
  disabled,
}: {
  providers: OIDCProvider[];
  value: string;
  onChange: (id: string) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation('auth');
  const directories = directoryProviders(providers);
  const current = directories.find((provider) => provider.id === value);

  return (
    <div>
      <p className="mb-1 text-sm font-medium text-foreground">{t('login.directory')}</p>
      {directories.length > 1 ? (
        <Select
          variant="outline"
          value={value}
          onChange={onChange}
          disabled={disabled}
          ariaLabel={t('login.directory')}
          options={directories.map((provider) => ({ value: provider.id, label: provider.name }))}
        />
      ) : (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <IconAddressBook className="size-4" aria-hidden="true" />
          {current?.name}
        </p>
      )}
    </div>
  );
}

export function SignInOptions({
  providers,
  directoryId,
  disabled,
  onRedirect,
  onDirectory,
  onLocal,
}: {
  providers: OIDCProvider[];
  directoryId: string | null;
  disabled: boolean;
  onRedirect: (provider: OIDCProvider) => void;
  onDirectory: (id: string) => void;
  onLocal: () => void;
}) {
  const { t } = useTranslation('auth');
  const directories = directoryProviders(providers);
  const redirects = providers.filter((provider) => provider.providerType !== 'ldap');
  const firstDirectory = directories[0];
  const offerDirectory = firstDirectory !== undefined && !directoryId;

  if (!directoryId && !offerDirectory && redirects.length === 0) return null;

  return (
    <>
      <div className="my-5 flex items-center gap-3">
        <div className="h-px flex-1 bg-border" />
        <span className="text-xs text-muted-foreground">{t('login.orSeparator')}</span>
        <div className="h-px flex-1 bg-border" />
      </div>

      <div className="space-y-2">
        {directoryId && (
          <Button variant="outline" className="w-full" onClick={onLocal} disabled={disabled}>
            <IconUser className="size-4" />
            {t('login.useLocalAccount')}
          </Button>
        )}
        {offerDirectory && (
          <Button
            variant="outline"
            className="w-full"
            onClick={() => onDirectory(firstDirectory.id)}
            disabled={disabled}
          >
            <IconAddressBook className="size-4" />
            {directories.length === 1
              ? t('login.signInWith', { provider: firstDirectory.name })
              : t('login.directoryAccount')}
          </Button>
        )}
        {redirects.map((provider) => {
          const Icon = PROVIDER_ICONS[provider.providerType];
          return (
            <Button
              key={provider.id}
              variant="outline"
              className="w-full"
              onClick={() => onRedirect(provider)}
              disabled={disabled}
            >
              {Icon && <Icon className="size-4" />}
              {t('login.signInWith', { provider: provider.name })}
            </Button>
          );
        })}
      </div>
    </>
  );
}
