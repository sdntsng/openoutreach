import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Campaign } from "../api";

export default function CampaignsPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .listCampaigns()
      .then((data) => setCampaigns(asArray(data, "campaigns")))
      .catch((err: Error) => setError(err.message));
  }, []);

  return (
    <div>
      <div className="row-actions" style={{ justifyContent: "space-between", marginBottom: "1rem" }}>
        <h1 style={{ margin: 0 }}>Campaigns</h1>
        <Link to="/campaigns/new">
          <button type="button">New campaign</button>
        </Link>
      </div>
      {error && <div className="error">{error}</div>}
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Leads</th>
            <th>Sent</th>
            <th>Replies</th>
            <th>Approx. opens</th>
            <th>Created</th>
          </tr>
        </thead>
        <tbody>
          {campaigns.length === 0 ? (
            <tr>
              <td colSpan={7} className="muted">
                No campaigns yet. Create a draft first — activate is a separate, explicit step.
              </td>
            </tr>
          ) : (
            campaigns.map((c) => (
              <tr key={String(c.id)}>
                <td>
                  <Link to={`/campaigns/${c.id}`}>{c.name}</Link>
                </td>
                <td>{c.status}</td>
                <td>{c.leads ?? "—"}</td>
                <td>{c.sent ?? 0}</td>
                <td>{c.replies ?? 0}</td>
                <td>{c.approx_opens ?? 0}</td>
                <td className="muted">{c.created_at ? new Date(c.created_at).toLocaleString() : "—"}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}
