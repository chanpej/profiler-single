import { AlertTriangle, X } from 'lucide-react';
import { useEffect } from 'react';

interface Props {
  message: string;
  open: boolean;
  onClose: () => void;
}

/** 自绘错误弹窗，替代顶部横幅与系统对话框。 */
export function ErrorPopup({ message, open, onClose }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open || !message) return null;

  return (
    <div
      className="error-popup-backdrop"
      data-testid="error-popup-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="error-popup" role="alertdialog" aria-modal="true" aria-labelledby="error-popup-title">
        <div className="error-popup-head">
          <AlertTriangle size={16} aria-hidden />
          <h2 id="error-popup-title">错误</h2>
          <button type="button" className="btn ghost" onClick={onClose} title="关闭" aria-label="关闭错误">
            <X size={14} aria-hidden />
          </button>
        </div>
        <p className="error-popup-body">{message}</p>
      </div>
    </div>
  );
}
