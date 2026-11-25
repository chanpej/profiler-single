import { useEffect, useRef, useState } from 'react';

export type MenuId = 'file' | 'view';

interface Props {
  onImport: () => void;
  onExport: () => void;
  onExportUnityData?: () => void;
  saving?: boolean;
  onClear: () => void;
  onFrames: () => void;
  onAnalysis: () => void;
}

/** 主菜单栏：侧边栏功能迁入，主界面始终保持采集视图。 */
export function MenuBar({
  onImport, onExport, onExportUnityData, saving, onClear, onFrames, onAnalysis,
}: Props) {
  const [open, setOpen] = useState<MenuId | null>(null);
  const rootRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(null);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(null);
    };
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const run = (fn: () => void) => {
    setOpen(null);
    fn();
  };

  const toggle = (id: MenuId) => {
    setOpen((cur) => (cur === id ? null : id));
  };

  return (
    <nav className="menubar" ref={rootRef} role="menubar" aria-label="主菜单" data-testid="menubar">
      <div className="menubar-item">
        <button
          type="button"
          className={`menubar-btn ${open === 'file' ? 'is-open' : ''}`}
          role="menuitem"
          aria-haspopup="true"
          aria-expanded={open === 'file'}
          onClick={() => toggle('file')}
          onMouseEnter={() => { if (open) setOpen('file'); }}
        >
          文件
        </button>
        {open === 'file' && (
          <div className="menubar-dropdown" role="menu">
            <button type="button" className="menubar-cmd" role="menuitem" onClick={() => run(onImport)}>
              加载数据
            </button>
            <button type="button" className="menubar-cmd" role="menuitem" disabled={saving} onClick={() => run(onExport)}>
              保存数据 (.upac)
            </button>
            {onExportUnityData && (
              <button type="button" className="menubar-cmd" role="menuitem" disabled={saving} onClick={() => run(onExportUnityData)}>
                导出 Unity .data
              </button>
            )}

            <button type="button" className="menubar-cmd" role="menuitem" onClick={() => run(onClear)}>
              清空数据
            </button>
          </div>
        )}
      </div>
      <div className="menubar-item">
        <button
          type="button"
          className={`menubar-btn ${open === 'view' ? 'is-open' : ''}`}
          role="menuitem"
          aria-haspopup="true"
          aria-expanded={open === 'view'}
          onClick={() => toggle('view')}
          onMouseEnter={() => { if (open) setOpen('view'); }}
        >
          视图
        </button>
        {open === 'view' && (
          <div className="menubar-dropdown" role="menu">
            <button type="button" className="menubar-cmd" role="menuitem" onClick={() => run(onFrames)}>
              帧数据
            </button>
          </div>
        )}
      </div>
      <button
        type="button"
        className="menubar-btn"
        role="menuitem"
        onClick={() => run(onAnalysis)}
        onMouseEnter={() => { if (open) setOpen(null); }}
      >
        诊断
      </button>
    </nav>
  );
}
