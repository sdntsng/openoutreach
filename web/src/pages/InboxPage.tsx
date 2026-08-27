import { useEffect, useState, type FormEvent } from "react";
import { api, asArray, type InboxThread, type ThreadMessage } from "../api";

export default function InboxPage() {
  const [threads, setThreads] = useState<InboxThread[]>([]);
  const [selected, setSelected] = useState<InboxThread | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [reply, setReply] = useState("");
  const [suggestion, setSuggestion] = useState<string>("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .listInbox()
      .then((data) => setThreads(asArray(data, "threads")))
      .catch((err: Error) => setError(err.message));
  }, []);

  useEffect(() => {
    if (!selected) {
      setMessages([]);
      return;
    }
    setError(null);
    api
      .getThread(selected.campaign_id, selected.lead_id)
      .then((d) => setMessages(d.messages || []))
      .catch((err: Error) => setError(err.message));
    api
      .suggestReply(selected.campaign_id, selected.lead_id)
      .then((d) => setSuggestion(d.suggested_body || ""))
      .catch(() => setSuggestion(""));
  }, [selected]);

  async function onReply(e: FormEvent) {
    e.preventDefault();
    if (!selected || !reply.trim() || !selected.contact) return;
    setBusy(true);
    setError(null);
    try {
      await api.replyToThread(
        selected.campaign_id,
        selected.lead_id,
        reply.trim(),
        selected.contact,
        true,
      );
      setReply("");
      const d = await api.getThread(selected.campaign_id, selected.lead_id);
      setMessages(d.messages || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <h1>Inbox</h1>
      {error && <div className="error">{error}</div>}
      <div className="split">
        <div className="thread-list">
          {threads.length === 0 ? (
            <p className="muted panel">No reply threads yet.</p>
          ) : (
            threads.map((t) => {
              const key = `${t.campaign_id}:${t.lead_id}`;
              const active =
                selected &&
                selected.campaign_id === t.campaign_id &&
                selected.lead_id === t.lead_id;
              return (
                <button
                  key={key}
                  type="button"
                  className={active ? "active" : undefined}
                  onClick={() => setSelected(t)}
                >
                  <div style={{ fontWeight: 500 }}>{t.subject || "(no subject)"}</div>
                  <div className="muted" style={{ fontSize: "0.8rem" }}>
                    {t.contact} · {t.campaign}
                  </div>
                </button>
              );
            })
          )}
        </div>
        <div>
          {!selected ? (
            <p className="muted panel">Select a thread.</p>
          ) : (
            <div>
              <h2 style={{ marginTop: 0 }}>{selected.subject || "Thread"}</h2>
              <p className="muted">{selected.contact}</p>
              {messages.map((m, i) => (
                <div className="message" key={m.id || i}>
                  <div className="meta">
                    {m.direction || "msg"} · {m.from_email || "—"} ·{" "}
                    {m.occurred_at ? new Date(m.occurred_at).toLocaleString() : ""}
                  </div>
                  <div style={{ whiteSpace: "pre-wrap" }}>
                    {m.display_body || m.text_body || ""}
                  </div>
                </div>
              ))}
              <form className="form-grid panel" onSubmit={onReply}>
                {suggestion ? (
                  <button
                    type="button"
                    className="secondary"
                    onClick={() => setReply(suggestion)}
                  >
                    Use suggested reply
                  </button>
                ) : null}
                <label>
                  Reply
                  <textarea
                    value={reply}
                    onChange={(e) => setReply(e.target.value)}
                    rows={6}
                    required
                  />
                </label>
                <button type="submit" disabled={busy}>
                  Send reply (same Gmail thread)
                </button>
              </form>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
