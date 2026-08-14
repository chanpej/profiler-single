import { X } from 'lucide-react';
import { useEffect } from 'react';

interface Props {
  title: string;
  open: boolean;
  onClose: () => void;
  children: React.ReactNode;
  wide?: boolean;
}

/** 覆盖在采集主界面上的对话框，不切换主视图。 */
export function AppDialog({ title, open, onClose, children, wide = false }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="dialog-backdrop"
      data-testid="dialog-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        className={`dialog ${wide ? 'is-wide' : ''}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="app-dialog-title"
      >
        <div className="dialog-head">
          <h2 id="app-dialog-title">{title}</h2>
          <button type="button" className="btn ghost" onClick={onClose} title="关闭对话框" aria-label="关闭对话框">
            <X size={14} aria-hidden />
          </button>
        </div>
        <div className="dialog-body">{children}</div>
      </div>
    </div>
  );
}
