import { useState } from 'react';
import { Loader2, ScanSearch } from 'lucide-react';
import type { Report } from '../types';
import { IssuesPanel } from './IssuesPanel';

interface Props {
  analyze: () => Promise<Report>;
}

/** 诊断视图：规则分析。 */
export function AnalysisView({ analyze }: Props) {
  const [report, setReport] = useState<Report | null>(null);
  const [analyzing, setAnalyzing] = useState(false);
  const [error, setError] = useState('');

  const runAnalyze = async () => {
    setAnalyzing(true);
    setError('');
    try {
      setReport(await analyze());
    } catch (e) {
      setError(String(e));
    } finally {
      setAnalyzing(false);
    }
  };

  return (
    <div className="analysis-view">
      <div className="panel">
        <div className="panel-head">
          <h2>规则分析</h2>
          <button type="button" className="btn primary" onClick={runAnalyze} disabled={analyzing} title="执行规则分析" aria-label="执行规则分析">
            {analyzing ? <Loader2 size={16} className="spin" aria-hidden /> : <ScanSearch size={16} aria-hidden />}
          </button>
        </div>
        {report ? (
          <>
            <p className="summary" data-testid="report-summary">{report.summary}</p>
            <IssuesPanel issues={report.issues} />
          </>
        ) : (
          <p className="dim">点击右上角按钮对当前会话执行规则化分析。</p>
        )}
      </div>

      {error && <div className="error-banner" role="alert">{error}</div>}
    </div>
  );
}
