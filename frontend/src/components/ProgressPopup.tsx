import { Loader2 } from 'lucide-react';
import { clampPercent, importProgressLabel } from '../lib/importProgress';
import type { ImportProgressEvent } from '../types';

interface Props {
  progress: ImportProgressEvent | null;
}

/** 加载存档时的自绘解析进度弹窗，不可手动关闭。 */
export function ProgressPopup({ progress }: Props) {
  if (!progress) return null;
  const percent = clampPercent(progress.percent);
  const title = importProgressLabel(progress);

  return (
    <div className="progress-popup-backdrop" data-testid="progress-popup-backdrop">
      <div
        className="progress-popup"
        role="dialog"
        aria-modal="true"
        aria-labelledby="progress-popup-title"
        aria-busy="true"
      >
        <div className="progress-popup-head">
          <Loader2 size={16} className="progress-popup-spin" aria-hidden />
          <h2 id="progress-popup-title">{title}</h2>
        </div>
        <div className="progress-popup-body">
          {progress.fileName ? <p className="progress-popup-file">{progress.fileName}</p> : null}
          <div
            className="progress-bar"
            role="progressbar"
            aria-label="解析进度"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percent}
          >
            <div className="progress-bar-fill" style={{ width: `${percent}%` }} />
          </div>
          <p className="progress-popup-pct mono">{percent}%</p>
        </div>
      </div>
    </div>
  );
}
