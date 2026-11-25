import { useEffect, useRef, useState } from 'react';
import { Camera } from 'lucide-react';

interface Thumbnail {
  id: string;
  frame: number;
  image: string;
  deltaMs: number;
  captureMs: number;
  streamed?: boolean;
}

export interface ScreenFilmstripData {
  enabled: boolean;
  available: boolean;
  error: string;
  thumbnails: Thumbnail[] | null;
  selected: Thumbnail | null;
}

interface Props {
  fetchStrip: (offset: number, count: number, selected: number) => Promise<ScreenFilmstripData>;
  wirelessAddress?: string;
  startWireless?: (address: string) => Promise<void>;
  enableWireless?: () => Promise<string>;
  setCapture: (enabled: boolean) => Promise<void>;
  connected?: boolean;
  start: number;
  count: number;
  selected: number;
  onSelect: (selected: number) => void;
}

const empty: ScreenFilmstripData = { enabled: false, available: false, error: '', thumbnails: [], selected: null };
const imageSource = (shot: Thumbnail) => `data:image/jpeg;base64,${shot.image}`;
const timing = (shot: Thumbnail) => `估算偏移 ${shot.deltaMs >= 0 ? '+' : ''}${shot.deltaMs} ms · ${shot.streamed ? '连续采集' : `截图耗时 ${shot.captureMs} ms`}`;

export function ScreenFilmstrip({ fetchStrip, setCapture, wirelessAddress, startWireless, enableWireless, connected, start, count, selected, onSelect }: Props) {
  const [strip, setStrip] = useState(empty);
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [wireless, setWireless] = useState(false);
  const [manualAddress, setAddress] = useState<string>();
  const address = manualAddress ?? wirelessAddress ?? '';
  const [preview, setPreview] = useState(false);
  const [error, setError] = useState('');
  const viewport = useRef({ start, count, selected });
  viewport.current = { start, count, selected };

  useEffect(() => {
    let disposed = false;
    let fetching = false;
    const refresh = async () => {
      if (fetching) return;
      fetching = true;
      const view = viewport.current;
      try {
        const result = await fetchStrip(view.start, open ? view.count : 0, open ? view.selected : -1);
        if (!disposed) setStrip(result);
      } catch {
        if (!disposed) setStrip(empty);
      } finally {
        fetching = false;
      }
    };
    void refresh();
    const timer = window.setInterval(refresh, open ? 250 : 750);
    return () => { disposed = true; window.clearInterval(timer); };
  }, [fetchStrip, open, connected, busy]);

  const toggle = async () => {
    setBusy(true);
    setError('');
    try {
      if (!strip.enabled && wireless && startWireless) await startWireless(address.trim());
      else await setCapture(!strip.enabled);
      if (!strip.enabled) setOpen(true);
    } catch (reason) {
      setError(String(reason));
    } finally {
      setBusy(false);
    }
  };

  const picked = strip.selected?.frame === selected ? strip.selected : null;
  const visible = (strip.thumbnails ?? []).filter((shot) => shot.frame >= start && shot.frame < start + count);
  return (
    <section className="screen-filmstrip" aria-label="画面轨道">
      <div className="screen-filmstrip-toolbar">
        <button type="button" className="btn ghost" onClick={() => setOpen(!open)} aria-expanded={open}>画面轨道</button>
        {startWireless && <label><input type="checkbox" checked={wireless} disabled={busy || strip.enabled}
          onChange={(event) => setWireless(event.target.checked)} />无线截图</label>}
        {wireless && <input className="input host" aria-label="无线 ADB 地址" placeholder="手机 IP:连接端口" value={address}
          disabled={busy || strip.enabled} onChange={(event) => setAddress(event.target.value)} />}
        {wireless && enableWireless && !connected && <button type="button" className="btn ghost" disabled={busy}
          title="在已授权的 USB 手机上开启 ADB TCP 5555 并填入 Wi-Fi 地址" onClick={async () => {
            setBusy(true); setError('');
            try { setAddress(await enableWireless()); } catch (reason) { setError(String(reason)); } finally { setBusy(false); }
          }}>通过 USB 开启无线 ADB</button>}
        <button type="button" className="btn ghost" onClick={toggle} disabled={busy || (!strip.enabled && (!connected || (wireless && !address.trim())))} aria-pressed={strip.enabled}>
          <Camera size={14} aria-hidden /> {strip.enabled ? '停止截图' : '采集截图'}
        </button>
        {open && picked && <button type="button" className="btn ghost" onClick={() => setPreview(!preview)}>{preview ? '关闭预览' : '预览选中帧'}</button>}
        {(error || strip.error) && <small role="alert">{error || strip.error}</small>}
      </div>
      {open && (
        <div className="screen-filmstrip-row">
          {visible.map((shot) => (
            <button type="button" key={shot.frame} aria-label={`跳到采集帧 ${shot.frame + 1}`} title={timing(shot)}
              className={picked?.id === shot.id ? 'is-selected' : ''} onClick={() => onSelect(shot.frame)}
              style={{ left: `clamp(0px, calc(${((shot.frame - start + 0.5) / count) * 100}% - 32px), calc(100% - 64px))` }}>
              <img src={imageSource(shot)} alt="" />
              <span className="mono">#{shot.frame + 1}</span>
            </button>
          ))}
          {visible.length === 0 && <small className="dim">当前帧范围暂无对齐截图</small>}
        </div>
      )}
      {open && preview && picked && (
        <figure className="screen-filmstrip-preview">
          <img src={imageSource(picked)} alt="选中帧附近的手机画面" />
          <figcaption>{timing(picked)}</figcaption>
        </figure>
      )}
    </section>
  );
}
