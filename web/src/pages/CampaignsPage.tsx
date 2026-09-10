import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Campaign } from "../api";
import { CampaignTable } from "../CampaignTable";
import { GATES, useWorkspace } from "../workspace";

export default function CampaignsPage() {
  const ws = useWorkspace();
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
      <div className="page-header">
        <h1>Campaigns</h1>
        {ws.hasSender ? (
          <Link to="/campaigns/new">
            <button type="button">New campaign</button>
          </Link>
        ) : (
          <Link to={GATES.sender.to}>
            <button type="button" className="secondary">
              Connect a sending account
            </button>
          </Link>
        )}
      </div>
      {error && <div className="error">{error}</div>}
      <CampaignTable campaigns={campaigns} />
    </div>
  );
}
