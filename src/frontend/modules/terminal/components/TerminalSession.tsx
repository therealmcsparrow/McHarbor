import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { useXTerm } from '@resources/components/XTermPanel';

type TerminalSessionProps = {
  containerId: string;
  active: boolean;
  onDisconnect: () => void;
};

export default function TerminalSession({ containerId, active, onDisconnect }: TerminalSessionProps) {
  const { t } = useTranslation('terminal');
  const { termRef, connected } = useXTerm(containerId, { autoConnect: active, active });

  return (
    <>
      <div
        ref={termRef}
        className="min-h-[400px] flex-1 rounded-lg border border-border bg-[#0a0a0a] p-1"
      />
      {connected && (
        <div className="flex justify-end">
          <Button variant="outline" onClick={onDisconnect}>
            {t('disconnect')}
          </Button>
        </div>
      )}
    </>
  );
}
