import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray, type Campaign } from "../api";
import { CampaignTable } from "../CampaignTable";

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
      <div className="page-header">
        <h1>Campaigns</h1>
        <Link to="/campaigns/new">
          <button type="button">New campaign</button>
        </Link>
      </div>
      {error && <div className="error">{error}</div>}
      <CampaignTable campaigns={campaigns} />
    </div>
  );
}
