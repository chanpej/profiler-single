import { useEffect, useState } from 'react';
import { Bot, Loader2, Plus, Save, Unplug } from 'lucide-react';
import { flushSync } from 'react-dom';
import type { LLMConfig, ProbeResult } from '../types';
import {
  applyChatFontSize,
  loadChatFontSize,
  MAX_CHAT_FONT_SIZE,
  MIN_CHAT_FONT_SIZE,
  saveChatFontSize,
} from '../lib/chatAppearance';
import {
  activeAgent,
  addCustomAgent,
  argsToText,
  normalizeLLMConfig,
  textToArgs,
  updateAgent,
  withActiveAgent,
} from '../lib/llmConfig';

export type SettingsGroupId = 'general' | 'agent';

interface Props {
  load: () => Promise<LLMConfig>;
  save: (cfg: LLMConfig) => Promise<void>;
  probe: (cfg: LLMConfig) => Promise<ProbeResult>;
}

/** 左侧分组设置：常规 / Agent（含 Codex、Cursor CLI 预设与连接检测）。 */
export function SettingsView({ load, save, probe }: Props) {
  const [group, setGroup] = useState<SettingsGroupId>('agent');
  const [cfg, setCfg] = useState<LLMConfig>(() => normalizeLLMConfig(null));
  const [saved, setSaved] = useState(false);
  const [probing, setProbing] = useState(false);
  const [probeMsg, setProbeMsg] = useState('');
  const [probeOk, setProbeOk] = useState<boolean | null>(null);
  const [chatFont, setChatFont] = useState(loadChatFontSize);

  useEffect(() => {
    load().then((raw) => setCfg(normalizeLLMConfig(raw)));
  }, [load]);

  const current = activeAgent(cfg);

  const onSave = async () => {
    await save(cfg);
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  const onChatFont = (n: number) => {
    setChatFont(n);
    saveChatFontSize(n);
    applyChatFontSize(n);
  };

  const onProbe = async () => {
    flushSync(() => {
      setProbing(true);
      setProbeMsg('正在检测 ACP 连接…');
      setProbeOk(null);
    });
    try {
      const r = await probe(cfg);
      setProbeOk(r.ok);
      setProbeMsg(r.message || (r.ok ? 'ACP 握手成功' : '检测失败'));
    } catch (e) {
      setProbeOk(false);
      setProbeMsg(String(e));
    } finally {
      setProbing(false);
    }
  };

  return (
    <div className="settings-layout" data-testid="settings-layout">
      <nav className="settings-nav" aria-label="设置分组">
        <button
          type="button"
          className={`settings-nav-item ${group === 'general' ? 'is-active' : ''}`}
          onClick={() => setGroup('general')}
        >
          常规
        </button>
        <button
          type="button"
          className={`settings-nav-item ${group === 'agent' ? 'is-active' : ''}`}
          onClick={() => setGroup('agent')}
        >
          Agent
        </button>
      </nav>
      <div className="settings-content">
        {group === 'general' && (
          <div className="settings-pane" data-testid="settings-general">
            <h2>常规</h2>
            <p className="dim">UnityPerfAgent 仅支持 Unity 2022 Development Build（测试包 2022.3.47f1）。</p>
            <label className="field">
              <span>Agent 聊天字号（{chatFont}px）</span>
              <input
                type="range"
                className="input-range"
                min={MIN_CHAT_FONT_SIZE}
                max={MAX_CHAT_FONT_SIZE}
                step={1}
                value={chatFont}
                aria-label="Agent 聊天字号"
                onChange={(e) => onChatFont(Number(e.target.value))}
              />
            </label>
          </div>
        )}
        {group === 'agent' && (
          <div className="settings-pane settings-view" data-testid="settings-agent">
            <div className="panel-head">
              <h2>ACP Agent</h2>
              <div className="settings-actions">
                <button
                  type="button"
                  className="btn ghost"
                  onClick={onProbe}
                  disabled={probing || current.command.trim() === ''}
                  title="检测 ACP 连接"
                  aria-label="检测 ACP 连接"
                  data-testid="probe-agent"
                >
                  {probing
                    ? <Loader2 size={14} className="spin" aria-hidden />
                    : <Unplug size={14} aria-hidden />}
                </button>
                <button type="button" className="btn primary" onClick={onSave} title="保存配置" aria-label="保存配置">
                  <Save size={16} aria-hidden />
                </button>
              </div>
            </div>
            <p className="dim">
              通过 ACP 接入外部 Agent。默认提供
              {' '}
              <code>Codex</code>
              （本进程用 acp-go-sdk 适配
              {' '}
              <code>codex exec</code>
              ）与
              {' '}
              <code>Cursor CLI</code>
              （
              <code>agent acp</code>
              ）。
            </p>
            {(probing || probeOk !== null) && (
              <div
                className={`probe-result ${probing ? 'is-busy' : probeOk ? 'is-ok' : 'is-bad'}`}
                role="status"
                data-testid="probe-result"
              >
                {probeMsg}
              </div>
            )}
            <div className="agent-preset-list" role="list" aria-label="Agent 列表">
              {cfg.agents.map((a) => (
                <button
                  key={a.id}
                  type="button"
                  role="listitem"
                  className={`agent-preset ${cfg.activeId === a.id ? 'is-active' : ''}`}
                  aria-label={a.name}
                  onClick={() => setCfg((c) => withActiveAgent(c, a.id))}
                >
                  <Bot size={12} aria-hidden />
                  {a.name}
                </button>
              ))}
              <button
                type="button"
                className="agent-preset"
                onClick={() => setCfg((c) => addCustomAgent(c))}
                title="添加自定义 Agent"
                aria-label="添加自定义 Agent"
              >
                <Plus size={12} aria-hidden />
              </button>
            </div>
            <label className="field">
              <span>名称</span>
              <input
                className="input"
                value={current.name}
                onChange={(e) => setCfg((c) => updateAgent(c, { id: current.id, name: e.target.value }))}
                spellCheck={false}
              />
            </label>
            <label className="field">
              <span>Agent 命令</span>
              <input
                className="input"
                value={current.command}
                onChange={(e) => setCfg((c) => updateAgent(c, { id: current.id, command: e.target.value }))}
                placeholder="codex 或 agent"
                spellCheck={false}
              />
            </label>
            <label className="field">
              <span>参数</span>
              <input
                className="input"
                value={argsToText(current.args)}
                onChange={(e) => setCfg((c) => updateAgent(c, { id: current.id, args: textToArgs(e.target.value) }))}
                placeholder="exec 或 acp"
                spellCheck={false}
              />
            </label>
            <label className="field">
              <span>工作目录（可选）</span>
              <input
                className="input"
                value={current.workDir}
                onChange={(e) => setCfg((c) => updateAgent(c, { id: current.id, workDir: e.target.value }))}
                placeholder="留空使用当前目录"
                spellCheck={false}
              />
            </label>
            {saved && <div className="saved-hint" role="status">已保存</div>}
          </div>
        )}
      </div>
    </div>
  );
}
