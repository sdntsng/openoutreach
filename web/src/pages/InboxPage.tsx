import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api, type InboxCounts, type InboxThread, type TeamMember, type ThreadMessage } from "../api";
import { isHot } from "../defaults";
import { PageIntro } from "../ui";
import { useWorkspace } from "../workspace";

type Box = "needs" | "replies" | "sent";

export default function InboxPage() {
  const ws = useWorkspace();
  const [params, setParams] = useSearchParams();
  const box = (params.get("box") as Box) || "needs";
  const [threads, setThreads] = useState<InboxThread[]>([]);
  const [counts, setCounts] = useState<InboxCounts>({ needs: 0, replies: 0, sent: 0 });
  const [selected, setSelected] = useState<InboxThread | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [owner, setOwner] = useState("");
  const [handoffStatus, setHandoffStatus] = useState("");
  const [deliveryId, setDeliveryId] = useState(0);
  const [campaigns, setCampaigns] = useState<string[]>([]);
  const [campaign, setCampaign] = useState("all");
  const [reply, setReply] = useState("");
  const [suggestion, setSuggestion] = useState("");
  const [suggestMeta, setSuggestMeta] = useState("");
  const [members, setMembers] = useState<TeamMember[]>([]);
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
    api
      .listTeam()
      .then((d) => setMembers(d.members || []))
      .catch(() => setMembers([]));
  }, [box]);

  useEffect(() => {
    if (!selected) {
      setMessages([]);
      setSuggestion("");
      setSuggestMeta("");
      setOwner("");
      setHandoffStatus("");
      setDeliveryId(0);
      return;
    }
    setError(null);
    api
      .getThread(selected.campaign_id, selected.lead_id)
      .then((d) => {
        setMessages(d.messages || []);
        setOwner(d.conversation?.owner_email || selected.owner_email || "");
        setHandoffStatus(d.conversation?.handoff_status || selected.handoff_status || "");
        setDeliveryId(d.conversation?.last_delivery_id || 0);
      })
      .catch((err: Error) => setError(err.message));
    if (box !== "sent") {
      api
        .suggestReply(selected.campaign_id, selected.lead_id)
        .then((d) => {
          setSuggestion(d.suggested_body || "");
          setSuggestMeta(
            d.used_playbook
              ? "Suggestion uses project company/offer plus the classification."
              : `Suggestion is canned text from classification (${d.source || d.classification || "unknown"}). You still send.`,
          );
        })
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
      if (next === "hot") {
        const res = await api.classifyThread(selected.campaign_id, selected.lead_id, next);
        setSelected({ ...selected, classification: next });
        setThreads((prev) =>
          prev.map((t) =>
            t.campaign_id === selected.campaign_id && t.lead_id === selected.lead_id
              ? { ...t, classification: next, handoff_status: res.handoff?.status || t.handoff_status }
              : t,
          ),
        );
        setHandoffStatus(res.handoff?.status || res.conversation?.handoff_status || "");
        setDeliveryId(res.handoff?.id || res.delivery_id || res.conversation?.last_delivery_id || 0);
      } else {
        await api.classifyThread(selected.campaign_id, selected.lead_id, next);
        setSelected({ ...selected, classification: next });
        setThreads((prev) =>
          prev.map((t) =>
            t.campaign_id === selected.campaign_id && t.lead_id === selected.lead_id
              ? { ...t, classification: next }
              : t,
          ),
        );
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function handoff() {
    if (!selected) return;
    setBusy(true);
    setError(null);
    try {
      const res = await api.handoffThread(selected.campaign_id, selected.lead_id, { owner_email: owner, confirm: true });
      setHandoffStatus(res.handoff?.status || "");
      setDeliveryId(res.delivery_id || res.handoff?.id || 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function retryHandoff() {
    if (!deliveryId) return;
    setBusy(true);
    setError(null);
    try {
      const d = await api.retryHandoff(deliveryId);
      setHandoffStatus(d.status || "");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function saveOwner() {
    if (!selected) return;
    setBusy(true);
    try {
      await api.patchThread(selected.campaign_id, selected.lead_id, { owner_email: owner });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <PageIntro title="Inbox">
        Own the conversation in the same thread you sent from. Marking interest queues a handoff —
        retry if delivery failed, without sending the outreach again.
      </PageIntro>
      {error && <div className="error">{error}</div>}
      {!ws.canReply ? (
        <p className="muted">
          In-thread replies need a mailbox that can receive mail.{" "}
          <Link to="/integrations?kind=send">Connect Gmail or Microsoft 365</Link>. Send-only providers cannot ingest
          replies here.
        </p>
      ) : null}
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
                    {t.handoff_status === "failed" ? <span className="badge badge-warn">Handoff failed</span> : null}
                  </div>
                  <div className="muted" style={{ fontSize: "0.8rem" }}>
                    {t.campaign} · {t.subject || "—"}
                    {t.owner_email ? ` · ${t.owner_email}` : ""}
                    {t.needs_action ? " · needs action" : ""}
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
              <div className="card stack" style={{ margin: "0.75rem 0" }}>
                <label>
                  Owner
                  <input
                    list="team-owners"
                    value={owner}
                    onChange={(e) => setOwner(e.target.value)}
                    placeholder="teammate@yourcompany.com"
                  />
                </label>
                <datalist id="team-owners">
                  {members.map((m) => (
                    <option key={m.email} value={m.email} />
                  ))}
                </datalist>
                <div className="row-actions">
                  <button type="button" className="secondary" disabled={busy} onClick={() => void saveOwner()}>
                    Save owner
                  </button>
                  <button type="button" disabled={busy} onClick={() => void handoff()}>
                    Route to teammate
                  </button>
                  {handoffStatus === "failed" && deliveryId ? (
                    <button type="button" className="secondary" disabled={busy} onClick={() => void retryHandoff()}>
                      Retry handoff
                    </button>
                  ) : null}
                </div>
                <p className="muted">
                  Handoff: {handoffStatus || "none"}
                  {handoffStatus === "failed"
                    ? " — delivery failed. Retry sends the same thread context, not another outreach email."
                    : null}
                  {handoffStatus === "success" ? " — delivered." : null}
                  {!ws.hasOutbound ? (
                    <>
                      {" "}
                      <Link to="/integrations?connect=outbound">Connect an outbound webhook</Link> so routing can succeed.
                    </>
                  ) : null}
                </p>
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
                    <>
                      <button type="button" className="secondary" onClick={() => setReply(suggestion)}>
                        Use suggested reply
                      </button>
                      {suggestMeta ? <p className="muted">{suggestMeta}</p> : null}
                    </>
                  ) : null}
                  <label>
                    Reply
                    <textarea value={reply} onChange={(e) => setReply(e.target.value)} rows={6} required />
                  </label>
                  <button type="submit" disabled={busy || !ws.canReply}>
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
