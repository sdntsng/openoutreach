import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { api, type InboxCounts, type InboxThread, type ThreadMessage } from "../api";
import { isHot } from "../defaults";

type Box = "needs" | "replies" | "sent";

export default function InboxPage() {
  const [params, setParams] = useSearchParams();
  const box = (params.get("box") as Box) || "needs";
  const [threads, setThreads] = useState<InboxThread[]>([]);
  const [counts, setCounts] = useState<InboxCounts>({ needs: 0, replies: 0, sent: 0 });
  const [selected, setSelected] = useState<InboxThread | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [campaigns, setCampaigns] = useState<string[]>([]);
  const [campaign, setCampaign] = useState("all");
  const [reply, setReply] = useState("");
  const [suggestion, setSuggestion] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setError(null);
    api
      .listInbox(box)
      .then((data) => {
        const list = data.threads || [];
        setThreads(list);
        if (data.counts) setCounts(data.counts);
        setCampaigns([...new Set(list.map((t) => t.campaign || "").filter(Boolean))]);
      })
      .catch((err: Error) => setError(err.message));
  }, [box]);

  useEffect(() => {
    if (!selected) {
      setMessages([]);
      setSuggestion("");
      return;
    }
    setError(null);
    api
      .getThread(selected.campaign_id, selected.lead_id)
      .then((d) => setMessages(d.messages || []))
      .catch((err: Error) => setError(err.message));
    if (box !== "sent") {
      api
        .suggestReply(selected.campaign_id, selected.lead_id)
        .then((d) => setSuggestion(d.suggested_body || ""))
        .catch(() => setSuggestion(""));
    }
  }, [selected, box]);

  const visible = useMemo(
    () => (campaign === "all" ? threads : threads.filter((t) => t.campaign === campaign)),
    [threads, campaign],
  );

  function setBox(next: Box) {
    const q = new URLSearchParams(params);
    if (next === "needs") q.delete("box");
    else q.set("box", next);
    setParams(q);
    setSelected(null);
  }

  async function onReply(e: FormEvent) {
    e.preventDefault();
    if (!selected || !reply.trim() || !selected.contact) return;
    setBusy(true);
    setError(null);
    try {
      await api.replyToThread(selected.campaign_id, selected.lead_id, reply.trim(), selected.contact, true);
      setReply("");
      const d = await api.getThread(selected.campaign_id, selected.lead_id);
      setMessages(d.messages || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function markHot() {
    if (!selected) return;
    setBusy(true);
    try {
      const next = isHot(selected.classification) ? "neutral" : "hot";
      await api.classifyThread(selected.campaign_id, selected.lead_id, next);
      setSelected({ ...selected, classification: next });
      setThreads((prev) =>
        prev.map((t) =>
          t.campaign_id === selected.campaign_id && t.lead_id === selected.lead_id
            ? { ...t, classification: next }
            : t,
        ),
      );
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
      <div className="tabs">
        {(
          [
            ["needs", `Needs reply (${counts.needs})`],
            ["replies", `Got reply (${counts.replies})`],
            ["sent", `Sent (${counts.sent})`],
          ] as const
        ).map(([id, label]) => (
          <button key={id} type="button" className={box === id ? "active" : undefined} onClick={() => setBox(id)}>
            {label}
          </button>
        ))}
      </div>
      <div className="row-actions" style={{ marginBottom: "0.85rem" }}>
        <label className="inline-field">
          Campaign
          <select value={campaign} onChange={(e) => setCampaign(e.target.value)}>
            <option value="all">All</option>
            {campaigns.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="split">
        <div className="thread-list">
          {visible.length === 0 ? (
            <p className="muted panel">
              {box === "needs"
                ? "Nothing waiting. Replies that still need a human response show up here."
                : box === "sent"
                  ? "No sent threads yet."
                  : "No reply threads yet."}
            </p>
          ) : (
            visible.map((t) => {
              const key = `${t.campaign_id}:${t.lead_id}`;
              const active =
                selected && selected.campaign_id === t.campaign_id && selected.lead_id === t.lead_id;
              return (
                <button key={key} type="button" className={active ? "active" : undefined} onClick={() => setSelected(t)}>
                  <div className="row-actions" style={{ justifyContent: "space-between" }}>
                    <div style={{ fontWeight: 500 }}>{t.contact || t.subject || "(no subject)"}</div>
                    {isHot(t.classification) ? <span className="badge badge-hot">Hot</span> : null}
                  </div>
                  <div className="muted" style={{ fontSize: "0.8rem" }}>
                    {t.campaign} · {t.subject || "—"}
                  </div>
                </button>
              );
            })
          )}
        </div>
        <div>
          {!selected ? (
            <p className="muted panel empty-pane">
              No conversation selected. Pick a contact from the list to read the thread and reply inline.
            </p>
          ) : (
            <div>
              <div className="row-actions" style={{ justifyContent: "space-between" }}>
                <div>
                  <h2 style={{ margin: 0 }}>{selected.contact || selected.subject || "Thread"}</h2>
                  <p className="muted" style={{ margin: "0.25rem 0 0" }}>
                    {selected.campaign}
                  </p>
                </div>
                {box !== "sent" ? (
                  <button type="button" className="secondary" disabled={busy} onClick={() => void markHot()}>
                    {isHot(selected.classification) ? "Clear Hot" : "Mark Hot"}
                  </button>
                ) : null}
              </div>
              {messages.map((m, i) => (
                <div className={`message ${m.direction === "outbound" ? "is-out" : ""}`} key={m.id || i}>
                  <div className="meta">
                    {m.direction || "msg"} · {m.from_email || "—"} ·{" "}
                    {m.occurred_at ? new Date(m.occurred_at).toLocaleString() : ""}
                  </div>
                  <div style={{ whiteSpace: "pre-wrap" }}>{m.display_body || m.text_body || ""}</div>
                </div>
              ))}
              {box !== "sent" ? (
                <form className="form-grid panel" onSubmit={onReply}>
                  {suggestion ? (
                    <button type="button" className="secondary" onClick={() => setReply(suggestion)}>
                      Use suggested reply
                    </button>
                  ) : null}
                  <label>
                    Reply
                    <textarea value={reply} onChange={(e) => setReply(e.target.value)} rows={6} required />
                  </label>
                  <button type="submit" disabled={busy}>
                    Send reply (same Gmail thread)
                  </button>
                </form>
              ) : null}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
