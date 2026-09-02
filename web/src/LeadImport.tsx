import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Campaign, type IntegrationCredential } from "./api";
import { CONNECTORS, connectorConnected, connectorEnabled } from "./connectors";
import { BrandMark, FileDrop, StatusBadge } from "./ui";

type Source = "csv" | "apollo" | "sheets" | "clay";

export function LeadImport({
  campaignId,
  campaigns,
  onImported,
}: {
  campaignId?: string | number;
  campaigns?: Campaign[];
  onImported?: () => void;
}) {
  const [source, setSource] = useState<Source>("csv");
  const [target, setTarget] = useState(campaignId != null ? String(campaignId) : "");
  const [csv, setCsv] = useState("email,first_name,company\n");
  const [fileName, setFileName] = useState("");
  const [apolloQ, setApolloQ] = useState("");
  const [apolloTitles, setApolloTitles] = useState("");
  const [apolloRows, setApolloRows] = useState<Record<string, string>[]>([]);
  const [sheetURL, setSheetURL] = useState("");
  const [creds, setCreds] = useState<IntegrationCredential[]>([]);
  const [caps, setCaps] = useState<Awaited<ReturnType<typeof api.capabilities>> | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const list = campaigns || [];
  const cid = campaignId ?? (target ? Number(target) || target : "");

  useEffect(() => {
    Promise.all([api.listIntegrations().catch(() => ({ integrations: [] })), api.capabilities().catch(() => null)])
      .then(([ints, c]) => {
        setCreds(asArray(ints, "integrations"));
        setCaps(c);
      })
      .catch(() => undefined);
  }, []);

  function connectorOf(id: string) {
    return CONNECTORS.find((c) => c.id === id);
  }

  function ready(id: string): boolean {
    const c = connectorOf(id);
    if (!c) return false;
    if (!connectorEnabled(c, caps)) return false;
    if (c.mode === "file") return true;
    return connectorConnected(c, [], creds);
  }

  async function importCSV(text: string) {
    if (cid === "" || cid == null) throw new Error("Pick a campaign to import into");
    const v = await api.validateLeads({ csv: text });
    if (v.invalid > 0) throw new Error(`Fix ${v.invalid} invalid row(s) before import`);
    await api.addLeads(cid, text);
    setNote(`Imported ${v.valid} lead${v.valid === 1 ? "" : "s"} into the campaign.`);
    onImported?.();
  }

  async function onCSV(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      await importCSV(csv);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card stack">
      <div>
        <h2 style={{ marginTop: 0 }}>Import leads</h2>
        <p className="muted">
          CSV file, or a connected source. Keys live on{" "}
          <Link to="/integrations">Integrations</Link>. Import never activates a campaign.
        </p>
      </div>
      {error && <div className="error">{error}</div>}
      {note && <p className="muted">{note}</p>}
      <div className="form-grid" style={{ maxWidth: "100%" }}>
        <label>
          Source
          <select value={source} onChange={(e) => setSource(e.target.value as Source)}>
            <option value="csv">CSV file</option>
            <option value="apollo">Apollo</option>
            <option value="sheets">Google Sheets</option>
            <option value="clay">Clay / webhook</option>
          </select>
        </label>
        {campaignId == null && (
          <label>
            Campaign
            <select value={target} onChange={(e) => setTarget(e.target.value)} required>
              <option value="">Select a campaign…</option>
              {list.map((c) => (
                <option key={String(c.id)} value={String(c.id)}>
                  {c.name} ({c.status})
                </option>
              ))}
            </select>
          </label>
        )}
        {list.length === 0 && campaignId == null && (
          <p className="muted">
            No campaigns yet. <Link to="/campaigns/new">Create a draft</Link> first, then import.
          </p>
        )}
      </div>

      {source === "csv" && (
        <form className="stack" onSubmit={(e) => void onCSV(e)}>
          <FileDrop
            label="Upload CSV"
            onText={(text, name) => {
              setCsv(text);
              setFileName(name);
            }}
          />
          {fileName && <p className="muted">Loaded {fileName}</p>}
          <label>
            Or paste CSV
            <textarea rows={8} value={csv} onChange={(e) => setCsv(e.target.value)} />
          </label>
          <button type="submit" disabled={busy || !csv.includes("@")}>
            Validate and import
          </button>
        </form>
      )}

      {source === "apollo" && (
        <ApolloImport
          ready={ready("apollo")}
          busy={busy}
          setBusy={setBusy}
          setError={setError}
          q={apolloQ}
          setQ={setApolloQ}
          titles={apolloTitles}
          setTitles={setApolloTitles}
          rows={apolloRows}
          setRows={setApolloRows}
          onImport={() => {
            if (cid === "" || cid == null) {
              setError("Pick a campaign to import into");
              return;
            }
            const text = [
              "email,first_name,last_name,company,domain,title,linkedin_url",
              ...apolloRows.map((r) =>
                [r.email, r.first_name, r.last_name, r.company, r.domain, r.title, r.linkedin_url]
                  .map((v) => `"${String(v || "").replaceAll('"', '""')}"`)
                  .join(","),
              ),
            ].join("\n");
            setBusy(true);
            importCSV(text)
              .catch((err: Error) => setError(err.message))
              .finally(() => setBusy(false));
          }}
        />
      )}

      {source === "sheets" && (
        <div className="stack">
          {!ready("sheets") ? (
            <NeedConnector id="sheets" />
          ) : (
            <>
              <label>
                Sheet or CSV URL
                <input
                  value={sheetURL}
                  onChange={(e) => setSheetURL(e.target.value)}
                  placeholder="https://docs.google.com/spreadsheets/d/…"
                />
              </label>
              <button
                type="button"
                disabled={busy || !sheetURL.trim() || cid === ""}
                onClick={() => {
                  if (cid === "" || cid == null) {
                    setError("Pick a campaign to import into");
                    return;
                  }
                  setBusy(true);
                  setError(null);
                  api
                    .sheetsImport({ url: sheetURL.trim(), campaign_id: Number(cid) })
                    .then(() => {
                      setNote("Sheet import finished.");
                      onImported?.();
                    })
                    .catch((err: Error) => setError(err.message))
                    .finally(() => setBusy(false));
                }}
              >
                Import from Sheet
              </button>
            </>
          )}
        </div>
      )}

      {source === "clay" && <ClayHint />}
    </div>
  );
}

function NeedConnector({ id }: { id: string }) {
  const c = CONNECTORS.find((x) => x.id === id);
  if (!c) return null;
  return (
    <div className="panel row-actions">
      <BrandMark connector={c} />
      <div>
        <strong>{c.name}</strong> is not connected.
        <div>
          <Link to={`/integrations?connect=${id}`}>Add the API key on Integrations</Link>
        </div>
      </div>
    </div>
  );
}

function ClayHint() {
  return (
    <div className="stack">
      <p className="muted">
        Clay and generic webhooks POST into a campaign. Save the HMAC on Integrations, then point
        Clay at the ingest URL. Ingest never activates.
      </p>
      <Link to="/integrations?connect=clay" className="row-actions">
        Set up Clay on Integrations
      </Link>
    </div>
  );
}

function ApolloImport({
  ready,
  busy,
  setBusy,
  setError,
  q,
  setQ,
  titles,
  setTitles,
  rows,
  setRows,
  onImport,
}: {
  ready: boolean;
  busy: boolean;
  setBusy: (v: boolean) => void;
  setError: (v: string | null) => void;
  q: string;
  setQ: (v: string) => void;
  titles: string;
  setTitles: (v: string) => void;
  rows: Record<string, string>[];
  setRows: (v: Record<string, string>[]) => void;
  onImport: () => void;
}) {
  if (!ready) return <NeedConnector id="apollo" />;
  return (
    <div className="stack">
      <label>
        Keywords
        <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="company, persona…" />
      </label>
      <label>
        Titles (optional)
        <input value={titles} onChange={(e) => setTitles(e.target.value)} placeholder="CRO, CMO" />
      </label>
      <button
        type="button"
        disabled={busy || !q.trim()}
        onClick={() => {
          setBusy(true);
          setError(null);
          const person_titles = titles
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean);
          api
            .apolloSearch({
              q_keywords: q.trim(),
              per_page: 10,
              person_titles: person_titles.length ? person_titles : undefined,
            })
            .then((res) => setRows(res.leads || []))
            .catch((err: Error) => setError(err.message))
            .finally(() => setBusy(false));
        }}
      >
        Search Apollo
      </button>
      {rows.length > 0 && (
        <>
          <p className="muted">{rows.length} preview rows — not imported yet</p>
          <table>
            <thead>
              <tr>
                <th>Email</th>
                <th>Name</th>
                <th>Company</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={`${r.email}-${i}`}>
                  <td>{r.email || "—"}</td>
                  <td>{[r.first_name, r.last_name].filter(Boolean).join(" ") || "—"}</td>
                  <td>{r.company || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <button type="button" disabled={busy} onClick={onImport}>
            Import preview
          </button>
        </>
      )}
    </div>
  );
}

export function YesNoBadge({ ok, yes = "Yes", no = "No" }: { ok: boolean; yes?: string; no?: string }) {
  return <StatusBadge ok={ok} on={yes} off={no} />;
}
