import { useState } from 'react';
import { Bot, Loader2, ScanSearch } from 'lucide-react';
import type { Report } from '../types';
import { IssuesPanel } from './IssuesPanel';

interface Props {
  analyze: () => Promise<Report>;
  diagnose: () => Promise<string>;
  llmReady: boolean;
}

/** 诊断视图：规则分析 + ACP LLM 智能诊断。 */
export function AnalysisView({ analyze, diagnose, llmReady }: Props) {
  const [report, setReport] = useState<Report | null>(null);
  const [analyzing, setAnalyzing] = useState(false);
  const [diagnosis, setDiagnosis] = useState('');
  const [diagnosing, setDiagnosing] = useState(false);
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

  const runDiagnose = async () => {
    setDiagnosing(true);
    setError('');
    setDiagnosis('');
    try {
      setDiagnosis(await diagnose());
    } catch (e) {
      setError(String(e));
    } finally {
      setDiagnosing(false);
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

      <div className="panel">
        <div className="panel-head">
          <h2>LLM 智能诊断</h2>
          <button
            type="button"
            className="btn primary"
            onClick={runDiagnose}
            disabled={diagnosing || !llmReady}
            title={llmReady ? '通过 ACP Agent 诊断' : '请先在设置中配置 ACP Agent'}
            aria-label="LLM 诊断"
          >
            {diagnosing ? <Loader2 size={16} className="spin" aria-hidden /> : <Bot size={16} aria-hidden />}
          </button>
        </div>
        {!llmReady && <p className="dim">尚未配置 ACP Agent，请前往设置页填写 Agent 命令。</p>}
        {diagnosing && <p className="dim">Agent 正在分析报告…</p>}
        {diagnosis && <pre className="diagnosis" data-testid="diagnosis">{diagnosis}</pre>}
      </div>

      {error && <div className="error-banner" role="alert">{error}</div>}
    </div>
  );
}
