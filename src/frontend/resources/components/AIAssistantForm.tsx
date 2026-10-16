// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBolt } from '@tabler/icons-react';
import { Button } from './ui/Button';
import { Spinner } from './ui/Spinner';
import { Input } from './ui/Input';

type PromptRecall = {
  recallPrevious: () => boolean;
  recallNext: () => boolean;
};

type AIAssistantFormProps = {
  input: string;
  setInput: (value: string) => void;
  inputRef: RefObject<HTMLInputElement | null>;
  pending: boolean;
  promptHistory: PromptRecall;
  onSend: () => void;
};

export function AIAssistantForm({ input, setInput, inputRef, pending, promptHistory, onSend }: AIAssistantFormProps) {
  const { t } = useTranslation(['ai', 'common']);
  return (
    <form
      className="flex w-full gap-2 px-4 py-3"
      onSubmit={(e) => {
        e.preventDefault();
        onSend();
      }}
    >
      <Input
        ref={inputRef}
        value={input}
        onChange={(e) => setInput(e.target.value)}
        placeholder={t('aiAssistant.placeholder')}
        aria-label={t('aiAssistant.placeholder')}
        className="flex-1 rounded-lg border border-border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:border-primary/40 focus:outline-none"
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            onSend();
            return;
          }
          // Shell-style recall. The field is single-line, so Up / Down
          // carry no other meaning here.
          if (e.key === 'ArrowUp' && promptHistory.recallPrevious()) {
            e.preventDefault();
          } else if (e.key === 'ArrowDown' && promptHistory.recallNext()) {
            e.preventDefault();
          }
        }}
      />
      <Button type="submit" disabled={pending || !input.trim()}>
        {pending ? <Spinner size="sm" /> : <IconBolt className="size-4" />}
        {t('aiAssistant.send')}
      </Button>
    </form>
  );
}
