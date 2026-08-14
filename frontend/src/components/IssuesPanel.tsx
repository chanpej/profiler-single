import { AlertTriangle, Info, OctagonAlert } from 'lucide-react';
import type { Issue, Severity } from '../types';

const severityIcon: Record<Severity, typeof Info> = {
  info: Info,
  warning: AlertTriangle,
  critical: OctagonAlert,
};

/** 规则分析结果列表。 */
export function IssuesPanel({ issues }: { issues: Issue[] }) {
  if (issues.length === 0) {
    return <div className="issues-empty" data-testid="issues-empty">未发现明显性能问题</div>;
  }
  return (
    <ul className="issues-list" data-testid="issues-list">
      {issues.map((issue, i) => {
        const Icon = severityIcon[issue.severity] ?? Info;
        return (
          <li key={i} className={`issue sev-${issue.severity}`}>
            <Icon size={16} aria-hidden className="issue-icon" />
            <div>
              <div className="issue-title">{issue.title}</div>
              <div className="issue-detail">{issue.detail}</div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}
