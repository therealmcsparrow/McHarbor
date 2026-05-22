// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import i18n, { changeAppLanguage, type SupportedLanguage } from '@core/i18n/i18n';

type LanguageState = {
  language: SupportedLanguage;
  setLanguage: (lang: SupportedLanguage) => void;
};

export const useLanguageStore = create<LanguageState>()(
  persist(
    (set) => ({
      language: (i18n.language?.substring(0, 2) as SupportedLanguage) || 'en',
      setLanguage: (lang) => {
        void changeAppLanguage(lang);
        set({ language: lang });
      },
    }),
    { name: 'mcharbor-language' }
  )
);
