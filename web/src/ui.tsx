import { Link } from "react-router-dom";
import type { Connector } from "./connectors";

export function BrandMark({ connector, size = 36 }: { connector: Connector; size?: number }) {
  return (
    <span
      className="brand-mark"
      style={{
        width: size,
        height: size,
        background: connector.color,
        fontSize: connector.mark.length > 2 ? 10 : 14,
      }}
      aria-hidden
    >
      {connector.mark}
    </span>
  );
}

export function StatusBadge({
  ok,
  on = "Connected",
  off = "Not connected",
}: {
  ok: boolean;
  on?: string;
  off?: string;
}) {
  return <span className={`badge ${ok ? "badge-ok" : "badge-off"}`}>{ok ? on : off}</span>;
}

export function SecretField({
  label,
  value,
  onChange,
  required,
  placeholder = "Paste key — stored encrypted, never shown again",
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  required?: boolean;
  placeholder?: string;
}) {
  return (
    <label>
      {label}
      <input
        className="secret-input"
        type="password"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required={required}
        autoComplete="new-password"
        spellCheck={false}
        placeholder={placeholder}
      />
    </label>
  );
}

export function FileDrop({
  label,
  accept = ".csv,text/csv,text/plain",
  onText,
}: {
  label: string;
  accept?: string;
  onText: (text: string, filename: string) => void;
}) {
  return (
    <label className="file-drop">
      {label}
      <input
        type="file"
        accept={accept}
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (!file) return;
          void file.text().then((text) => onText(text, file.name));
          e.target.value = "";
        }}
      />
      <span className="muted">Choose a .csv file or drop it here</span>
    </label>
  );
}

export function ConnectorCard({
  connector,
  enabled,
  connected,
  href,
  onOpen,
}: {
  connector: Connector;
  enabled: boolean;
  connected: boolean;
  href?: string;
  onOpen?: () => void;
}) {
  const inner = (
    <>
      <BrandMark connector={connector} />
      <div className="connector-copy">
        <div className="connector-name">{connector.name}</div>
        <p className="muted">{connector.blurb}</p>
      </div>
      <StatusBadge
        ok={enabled && connected}
        on={connector.mode === "file" ? "Ready" : "Connected"}
        off={enabled ? "Not connected" : "Off"}
      />
    </>
  );
  if (href) {
    return (
      <Link className={`connector-card ${enabled ? "" : "is-off"}`} to={href}>
        {inner}
      </Link>
    );
  }
  return (
    <button
      type="button"
      className={`connector-card ${enabled ? "" : "is-off"}`}
      onClick={onOpen}
      disabled={!enabled}
    >
      {inner}
    </button>
  );
}
