import { useState } from 'react';
import { CheckCircle2, Loader2 } from 'lucide-react';
import { clampPercent, fileBaseName, importProgressLabel } from '../lib/importProgress';
import type { ImportProgressEvent } from '../types';

export interface SaveProgress {
  path: string;
  percent: number | null;
}

interface Props {
  progress: ImportProgressEvent | null;
  save?: SaveProgress | null;
  onReveal?: (path: string) => Promise<void>;
  onClose?: () => void;
}

export function ProgressPopup({ progress, save, onClose, onReveal }: Props) {
  const [revealError, setRevealError] = useState('');
  if (!progress && !save) return null;
  const percent = save ? save.percent : clampPercent(progress?.percent ?? 0);
  const complete = save?.percent === 100;
  const title = complete ? '保存完成' : save
    ? (percent === null ? 'Unity 正在导出 .data…' : '正在保存存档…')
    : (progress ? importProgressLabel(progress) : '');
  const fileName = save ? fileBaseName(save.path) : progress?.fileName;

  return (
    <div className="progress-popup-backdrop" data-testid="progress-popup-backdrop">
      <div
        className="progress-popup"
        role="dialog"
        aria-modal="true"
        aria-labelledby="progress-popup-title"
        aria-busy={!complete}
      >
        <div className="progress-popup-head">
          {complete ? <CheckCircle2 size={16} aria-hidden /> : <Loader2 size={16} className="progress-popup-spin" aria-hidden />}
          <h2 id="progress-popup-title">{title}</h2>
        </div>
        <div className="progress-popup-body">
          {complete ? (
            <>
              <p className="progress-popup-path" role="status">{save?.path}</p>
              <div className="progress-popup-actions">
                {onReveal && <button type="button" className="btn" onClick={async () => {
                  setRevealError('');
                  try { await onReveal(save.path); } catch (error) { setRevealError(String(error)); }
                }}>打开文件夹</button>}
                <button type="button" className="btn primary" onClick={onClose} autoFocus>确定</button>
              </div>
              {revealError && <p role="alert">{revealError}</p>}
            </>
          ) : (
            <>
              {fileName && <p className="progress-popup-file">{fileName}</p>}
              <div className="progress-bar" role="progressbar" aria-label={save ? '保存进度' : '解析进度'}
                aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent ?? undefined}>
                <div className={`progress-bar-fill${percent === null ? ' is-indeterminate' : ''}`}
                  style={percent === null ? undefined : { width: `${percent}%` }} />
              </div>
              {percent !== null && <p className="progress-popup-pct mono">{percent}%</p>}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
