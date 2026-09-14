import { Link } from "react-router-dom";
import type { Campaign } from "./api";
import { replyRateLabel } from "./defaults";
import { StatusChip } from "./ui";

export function CampaignTable({ campaigns, empty }: { campaigns: Campaign[]; empty?: string }) {
  return (
    <table>
      <thead>
        <tr>
          <th>Campaign</th>
          <th>Status</th>
          <th>Sent</th>
          <th>Reply rate</th>
          <th>Interested</th>
          <th>Leads</th>
          <th>Approx. opens</th>
        </tr>
      </thead>
      <tbody>
        {campaigns.length === 0 ? (
          <tr>
            <td colSpan={7} className="muted">
              {empty || "No campaigns yet. Prepare a draft first — activate is a separate, explicit step."}
            </td>
          </tr>
        ) : (
          campaigns.map((c) => (
            <tr key={String(c.id)}>
              <td>
                <Link to={`/campaigns/${c.id}`}>{c.name}</Link>
              </td>
              <td>
                <StatusChip status={c.status} />
                {c.status === "active" && c.next_send ? (
                  <div className="muted" style={{ fontSize: "0.8rem" }}>
                    Next send {new Date(c.next_send).toLocaleString()}
                  </div>
                ) : c.status === "draft" ? (
                  <div className="muted" style={{ fontSize: "0.8rem" }}>
                    Nothing sends until you activate
                  </div>
                ) : null}
              </td>
              <td>{c.sent ?? 0}</td>
              <td>{c.sent ? replyRateLabel(c.reply_rate) : "—"}</td>
              <td>{c.interested ?? 0}</td>
              <td>{c.leads ?? "—"}</td>
              <td>{c.approx_opens ?? 0}</td>
            </tr>
          ))
        )}
      </tbody>
    </table>
  );
}
