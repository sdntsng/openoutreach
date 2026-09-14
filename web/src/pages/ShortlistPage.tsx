import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api, asArray, type Campaign, type ShortlistRow, type WorkspacePlaybook } from "../api";
import { LeadImport } from "../LeadImport";
import { PageIntro } from "../ui";

export default function ShortlistPage() {
  const [params] = useSearchParams();
  const campaignQ = Number(params.get("campaign") || "0");
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [campaignId, setCampaignId] = useState(0);
  const [rows, setRows] = useState<ShortlistRow[]>([]);
  const [playbook, setPlaybook] = useState<WorkspacePlaybook | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState("");
  const [editing, setEditing] = useState<Record<number, string>>({});

  async function loadCampaigns() {
    const c = asArray(await api.listCampaigns(), "campaigns");
    setCampaigns(c);
    const ids = c.map((x) => Number(x.id));
    setCampaignId((cur) => {
      if (campaignQ && ids.includes(campaignQ)) return campaignQ;
      if (cur && ids.includes(cur)) return cur;
      return Number(c[0]?.id || 0);
    });
  }

  async function loadList(cid: number) {
    if (!cid) {
      setRows([]);
      return;
    }
    const r = await api.listShortlist(cid);
    setRows(r.candidates || []);
    setPlaybook(r.playbook || null);
  }

  useEffect(() => {
    loadCampaigns().catch((e) => setErr(String(e)));
  }, [campaignQ]);

  useEffect(() => {
    if (!campaignId) return;
    loadList(campaignId).catch((e) => setErr(String(e)));
  }, [campaignId]);

  async function patch(id: number, body: { status?: string; fit_reason?: string }) {
    setBusy(String(id) + (body.status || "fit"));
    setErr("");
    try {
      await api.patchShortlist(id, body);
      await loadList(campaignId);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy("");
    }
  }

  async function enroll() {
    setBusy("enroll");
    setErr("");
    try {
      await api.enrollShortlist(campaignId, { all_approved: true });
      await loadList(campaignId);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy("");
    }
  }

  const pending = rows.filter((r) => r.status === "pending").length;
  const approved = rows.filter((r) => r.status === "approved").length;
  const fitLine = [playbook?.audience, playbook?.geography, playbook?.company_size].filter(Boolean).join(" · ");

  return (
    <div>
      <PageIntro title="Shortlist">
        Review why each person belongs before they enter a campaign. Approve or exclude first. Enrollment schedules
        mail only after that — it still does not send until you activate.
      </PageIntro>
      {err ? <p className="error">{err}</p> : null}
      {!campaigns.length ? (
        <p className="muted">
          Create a campaign first, then import people here. <Link to="/campaigns/new">New campaign</Link>
        </p>
      ) : (
        <>
          {fitLine ? (
            <p className="card" style={{ marginBottom: "1rem" }}>
              Fit we are looking for: {fitLine}
              {playbook?.offer ? ` · Offer: ${playbook.offer}` : ""}
            </p>
          ) : (
            <p className="muted">
              Set audience and offer on <Link to="/project">Project</Link> so reviewers know what “fits” means. Do
              not invent a score.
            </p>
          )}
          <label>
            Campaign
            <select value={campaignId} onChange={(e) => setCampaignId(Number(e.target.value))}>
              {campaigns.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name} (#{c.id})
                </option>
              ))}
            </select>
          </label>
          <p className="muted">
            {pending} awaiting review · {approved} approved ·{" "}
            <Link to={`/campaigns/${campaignId}`}>Open campaign review</Link>
          </p>
          <LeadImport campaignId={campaignId} onImported={() => loadList(campaignId)} destination="shortlist" />
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Person</th>
                  <th>Role / company</th>
                  <th>Why they fit</th>
                  <th>Source</th>
                  <th>Email check</th>
                  <th>Previous outreach</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.id}>
                    <td>
                      {r.first_name} {r.last_name}
                      <div className="muted" style={{ fontSize: 12 }}>
                        {r.email}
                      </div>
                    </td>
                    <td>
                      {r.title || "—"}
                      <div className="muted">{r.company}</div>
                    </td>
                    <td style={{ maxWidth: 280 }}>
                      {r.status === "pending" ? (
                        <textarea
                          rows={3}
                          value={editing[r.id] ?? r.fit_reason}
                          onChange={(e) => setEditing((prev) => ({ ...prev, [r.id]: e.target.value }))}
                          placeholder="Add why this person fits before you approve."
                        />
                      ) : (
                        r.fit_reason || "—"
                      )}
                    </td>
                    <td>
                      {r.source || "—"}
                      <div className="muted" style={{ fontSize: 12 }}>
                        {r.source_at ? new Date(r.source_at).toLocaleString() : ""}
                      </div>
                    </td>
                    <td>
                      {r.email_check}
                      {r.email_check_reason ? (
                        <div className="muted" style={{ fontSize: 12 }}>
                          {r.email_check_reason}
                        </div>
                      ) : null}
                    </td>
                    <td>{r.previous_outreach}</td>
                    <td>
                      {r.status === "pending" ? (
                        <span className="row-actions">
                          <button
                            type="button"
                            disabled={!!busy}
                            onClick={() =>
                              patch(r.id, {
                                status: "approved",
                                fit_reason: editing[r.id] ?? r.fit_reason,
                              })
                            }
                          >
                            Approve
                          </button>
                          <button
                            type="button"
                            className="secondary"
                            disabled={!!busy}
                            onClick={() => patch(r.id, { status: "excluded" })}
                          >
                            Exclude
                          </button>
                        </span>
                      ) : (
                        <span className="pill">{r.status}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {approved ? (
            <p>
              <button type="button" disabled={busy === "enroll"} onClick={() => void enroll()}>
                Enroll {approved} approved {approved === 1 ? "person" : "people"}
              </button>
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}
