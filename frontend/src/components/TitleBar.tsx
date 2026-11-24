import { Minus, Square, Copy, X } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { createWindowControls, type WindowControls } from '../lib/window';

interface Props {
  title?: string;
  subtitle?: string;
  controls?: WindowControls;
}

/** 自绘标题栏：拖拽、双击最大化、最小化/最大化/关闭。 */
export function TitleBar({ title = 'unityperf', subtitle = '', controls }: Props) {
  const win = controls ?? defaultControls;
  const [maximised, setMaximised] = useState(false);

  const refresh = useCallback(() => {
    win.isMaximised().then(setMaximised).catch(() => setMaximised(false));
  }, [win]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <header className="titlebar" data-testid="titlebar">
      <div
        className="titlebar-drag"
        onDoubleClick={() => {
          win.toggleMaximise().then(refresh).catch(() => {});
        }}
      >
        <img className="titlebar-icon" src="/app-icon.png" alt="" aria-hidden="true" data-testid="titlebar-icon" />
        <span className="titlebar-title">{title}</span>
        {subtitle ? <span className="titlebar-sub">{subtitle}</span> : null}
      </div>
      <div className="titlebar-controls">
        <button
          type="button"
          className="titlebar-btn"
          title="最小化"
          aria-label="最小化"
          onClick={() => win.minimise().catch(() => {})}
        >
          <Minus size={14} aria-hidden />
        </button>
        <button
          type="button"
          className="titlebar-btn"
          title={maximised ? '还原' : '最大化'}
          aria-label={maximised ? '还原' : '最大化'}
          onClick={() => win.toggleMaximise().then(refresh).catch(() => {})}
        >
          {maximised ? <Copy size={12} aria-hidden /> : <Square size={12} aria-hidden />}
        </button>
        <button
          type="button"
          className="titlebar-btn titlebar-btn-close"
          title="关闭"
          aria-label="关闭"
          onClick={() => win.close().catch(() => {})}
        >
          <X size={14} aria-hidden />
        </button>
      </div>
    </header>
  );
}

const defaultControls = createWindowControls();
