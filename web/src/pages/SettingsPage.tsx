import { useEffect, useState } from "react";
import { api } from "../api";

export default function SettingsPage() {
  const [workspaceId, setWorkspaceId] = useState<string>("…");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .workspace()
      .then((w) => setWorkspaceId(w.workspace_id || "default"))
      .catch((err: Error) => {
        setWorkspaceId("default");
        setError(err.message);
      });
  }, []);

  return (
    <div>
      <h1>Settings</h1>
      {error && (
        <div className="error">
          Could not load workspace from API ({error}). Showing fallback.
        </div>
      )}
      <div className="panel form-grid">
        <label>
          Workspace ID
          <input readOnly value={workspaceId} />
        </label>
        <p className="muted">
          Hosted requests resolve workspace from server config (
          <code>OPENOUTREACH_WORKSPACE_ID</code>), not from email domain.
        </p>
      </div>
    </div>
  );
}
