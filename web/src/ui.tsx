import type { ReactNode } from "react";
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
  hint,
  accept = ".csv,text/csv,text/plain",
  onText,
}: {
  label: string;
  hint?: string;
  accept?: string;
  onText: (text: string, filename: string) => void;
}) {
  function take(file: File | undefined) {
    if (!file) return;
    void file.text().then((text) => onText(text, file.name));
  }
  return (
    <label
      className="file-drop"
      onDragOver={(e) => {
        e.preventDefault();
      }}
      onDrop={(e) => {
        e.preventDefault();
        take(e.dataTransfer.files?.[0]);
      }}
    >
      <strong>{label}</strong>
      <input
        type="file"
        accept={accept}
        onChange={(e) => {
          take(e.target.files?.[0]);
          e.target.value = "";
        }}
      />
      <span className="muted">{hint || "Drop CSV or click to upload"}</span>
    </label>
  );
}

export function PageIntro({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="page-intro">
      <h1>{title}</h1>
      {children ? <p className="muted page-lede">{children}</p> : null}
    </div>
  );
}

export function StatusChip({ status }: { status: string }) {
  const s = (status || "").toLowerCase();
  const kind = s === "active" ? "ok" : s === "paused" ? "warn" : s === "draft" ? "off" : "off";
  return <span className={`badge badge-${kind}`}>{status || "—"}</span>;
}

export function PillList({
  items,
  onRemove,
}: {
  items: string[];
  onRemove?: (value: string) => void;
}) {
  if (!items.length) return null;
  return (
    <div className="pill-row">
      {items.map((item) => (
        <span key={item} className="pill">
          {item}
          {onRemove ? (
            <button type="button" className="pill-x" onClick={() => onRemove(item)} aria-label={`Remove ${item}`}>
              ×
            </button>
          ) : null}
        </span>
      ))}
    </div>
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
