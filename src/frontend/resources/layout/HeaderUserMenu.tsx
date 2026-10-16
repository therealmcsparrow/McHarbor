// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { IconInfoCircle, IconLogout, IconRefresh, IconUserCircle } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { Button } from '@resources/components/ui/Button';

type HeaderUserMenuProps = {
  username: string;
  menuOpen: boolean;
  onMenuOpenChange: (next: boolean) => void;
  canRestartMcHarbor: boolean;
  onRestart: () => void;
  onLogout: () => void;
};

const MENU_ITEM_CLASS = 'w-full justify-start rounded-none px-3 py-1.5 text-sm';

export function HeaderUserMenu({
  username,
  menuOpen,
  onMenuOpenChange,
  canRestartMcHarbor,
  onRestart,
  onLogout,
}: HeaderUserMenuProps) {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onMenuOpenChange(false);
      }
    }
    if (menuOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [menuOpen, onMenuOpenChange]);

  return (
    <div className="relative" ref={menuRef}>
      <Button variant="ghost" size="icon" onClick={() => onMenuOpenChange(!menuOpen)} aria-label={t('header.userMenu')}>
        <div className="flex size-7 items-center justify-center bg-primary text-xs font-medium text-primary-foreground">
          {username.charAt(0).toUpperCase()}
        </div>
      </Button>

      {menuOpen && (
        <div className="mc-glass-overlay mc-menu absolute top-full right-0 z-50 mt-1 w-52 overflow-hidden py-1">
          <div className="border-b border-border px-3 py-2.5">
            <div className="flex items-center gap-2.5">
              <div className="flex size-8 items-center justify-center bg-primary text-sm font-medium text-primary-foreground">
                {username.charAt(0).toUpperCase()}
              </div>
              <div>
                <p className="text-sm font-medium text-foreground">{username}</p>
                <p className="text-xs text-muted-foreground">{t('header.administrator')}</p>
              </div>
            </div>
          </div>

          <div className="my-1 border-t border-border" />
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              onMenuOpenChange(false);
              void navigate('/profile');
            }}
            className={MENU_ITEM_CLASS}
          >
            <IconUserCircle className="size-4 text-muted-foreground" />
            <span>{t('profile.menuItem')}</span>
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              onMenuOpenChange(false);
              void navigate('/settings?tab=about');
            }}
            className={MENU_ITEM_CLASS}
          >
            <IconInfoCircle className="size-4 text-muted-foreground" />
            <span>{t('about.menuItem')}</span>
          </Button>
          {canRestartMcHarbor && (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                onMenuOpenChange(false);
                onRestart();
              }}
              className={MENU_ITEM_CLASS}
              data-testid="restart-mcharbor-menu-item"
            >
              <IconRefresh className="size-4 text-muted-foreground" />
              <span>{t('system.menu.restartMcHarbor')}</span>
            </Button>
          )}
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              onMenuOpenChange(false);
              onLogout();
            }}
            className={`${MENU_ITEM_CLASS} text-destructive hover:text-destructive`}
          >
            <IconLogout className="size-4" />
            <span>{t('actions.signOut')}</span>
          </Button>
        </div>
      )}
    </div>
  );
}
