import { useEffect, useState } from 'react';
import { Save } from 'lucide-react';
import type { LLMConfig } from '../types';

interface Props {
  load: () => Promise<LLMConfig>;
  save: (cfg: LLMConfig) => Promise<void>;
}

/** 设置视图：ACP LLM Agent 接入配置。 */
export function SettingsView({ load, save }: Props) {
  const [command, setCommand] = useState('');
  const [args, setArgs] = useState('');
  const [workDir, setWorkDir] = useState('');
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    load().then((cfg) => {
      setCommand(cfg.agentCommand ?? '');
      setArgs((cfg.agentArgs ?? []).join(' '));
      setWorkDir(cfg.workDir ?? '');
    });
  }, [load]);

  const onSave = async () => {
    await save({
      agentCommand: command.trim(),
      agentArgs: args.trim() === '' ? [] : args.trim().split(/\s+/),
      workDir: workDir.trim(),
    });
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  return (
    <div className="settings-view panel">
      <div className="panel-head">
        <h2>ACP Agent 配置</h2>
        <button type="button" className="btn primary" onClick={onSave} title="保存配置" aria-label="保存配置">
          <Save size={16} aria-hidden />
        </button>
      </div>
      <p className="dim">
        通过 ACP（Agent Client Protocol）接入任意 LLM Agent，例如 <code>gemini --experimental-acp</code>。
      </p>
      <label className="field">
        <span>Agent 命令</span>
        <input className="input" value={command} onChange={(e) => setCommand(e.target.value)} placeholder="gemini" spellCheck={false} />
      </label>
      <label className="field">
        <span>参数</span>
        <input className="input" value={args} onChange={(e) => setArgs(e.target.value)} placeholder="--experimental-acp" spellCheck={false} />
      </label>
      <label className="field">
        <span>工作目录（可选）</span>
        <input className="input" value={workDir} onChange={(e) => setWorkDir(e.target.value)} placeholder="留空使用当前目录" spellCheck={false} />
      </label>
      {saved && <div className="saved-hint" role="status">已保存</div>}
    </div>
  );
}
