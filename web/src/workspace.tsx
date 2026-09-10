import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { api, asArray, type Account, type Capabilities, type IntegrationCredential, type SetupStatus } from "./api";
import { CONNECTORS, connectorConnected, connectorEnabled } from "./connectors";

export type GateId = "sender" | "apollo" | "sheets" | "clay" | "outbound" | "mcp";

export interface WorkspaceState {
  ready: boolean;
  accounts: Account[];
  creds: IntegrationCredential[];
  caps: Capabilities | null;
  setup: SetupStatus | null;
  hasSender: boolean;
  hasApollo: boolean;
  hasSheets: boolean;
  hasClay: boolean;
  hasOutbound: boolean;
  hasMcp: boolean;
  refresh: () => void;
}

const empty: Omit<WorkspaceState, "refresh"> = {
  ready: false,
  accounts: [],
  creds: [],
  caps: null,
  setup: null,
  hasSender: false,
  hasApollo: false,
  hasSheets: false,
  hasClay: false,
  hasOutbound: false,
  hasMcp: false,
};

const Ctx = createContext<WorkspaceState>({ ...empty, refresh: () => undefined });

function derive(
  accounts: Account[],
  creds: IntegrationCredential[],
  caps: Capabilities | null,
  setup: SetupStatus | null,
): Omit<WorkspaceState, "refresh"> {
  const find = (id: string) => CONNECTORS.find((c) => c.id === id);
  const on = (id: string) => {
    const c = find(id);
    if (!c) return false;
    if (!connectorEnabled(c, caps)) return false;
    return connectorConnected(c, accounts, creds);
  };
  return {
    ready: true,
    accounts,
    creds,
    caps,
    setup,
    hasSender: accounts.length > 0,
    hasApollo: on("apollo"),
    hasSheets: on("sheets"),
    hasClay: on("clay") || on("webhook"),
    hasOutbound: on("outbound"),
    hasMcp: Boolean(caps?.mcp_configured),
  };
}

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [tick, setTick] = useState(0);
  const [state, setState] = useState<Omit<WorkspaceState, "refresh">>(empty);

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      api.listAccounts().catch(() => ({ accounts: [] as Account[] })),
      api.listIntegrations().catch(() => ({ integrations: [] as IntegrationCredential[] })),
      api.capabilities().catch(() => null),
      api.setup().catch(() => null),
    ]).then(([acc, ints, caps, setup]) => {
      if (cancelled) return;
      setState(derive(asArray(acc, "accounts"), asArray(ints, "integrations"), caps, setup));
    });
    return () => {
      cancelled = true;
    };
  }, [tick]);

  const value = useMemo<WorkspaceState>(
    () => ({ ...state, refresh: () => setTick((n) => n + 1) }),
    [state],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useWorkspace(): WorkspaceState {
  return useContext(Ctx);
}

export const GATES: Record<
  GateId,
  { title: string; ask: string; to: string; connectorId?: string }
> = {
  sender: {
    title: "Connect a sending account",
    ask: "Sequences send from a mailbox you own. Google, Microsoft 365, or SMTP first — we never send through Instantly or Smartlead.",
    to: "/integrations?kind=send",
    connectorId: "gmail",
  },
  apollo: {
    title: "Connect Apollo",
    ask: "People search needs an Apollo API key. Preview stays a draft import — it never activates a campaign.",
    to: "/integrations?connect=apollo",
    connectorId: "apollo",
  },
  sheets: {
    title: "Connect Google Sheets",
    ask: "Paste a published sheet URL after you save the Sheets integration, or use CSV instead.",
    to: "/integrations?connect=sheets",
    connectorId: "sheets",
  },
  clay: {
    title: "Connect Clay",
    ask: "Clay posts into a campaign over a signed webhook. Save the HMAC, then point Clay at the ingest URL.",
    to: "/integrations?connect=clay",
    connectorId: "clay",
  },
  outbound: {
    title: "Connect an outbound webhook",
    ask: "Route replies and bounces to Slack, Make, or a CRM workflow. Failures never block send.",
    to: "/integrations?connect=outbound",
    connectorId: "outbound",
  },
  mcp: {
    title: "Set an MCP bearer",
    ask: "Agents use Authorization: Bearer on the MCP endpoint. The token is operator-set — it is never shown here.",
    to: "/settings",
  },
};

export function gateReady(ws: WorkspaceState, id: GateId): boolean {
  switch (id) {
    case "sender":
      return ws.hasSender;
    case "apollo":
      return ws.hasApollo;
    case "sheets":
      return ws.hasSheets;
    case "clay":
      return ws.hasClay;
    case "outbound":
      return ws.hasOutbound;
    case "mcp":
      return ws.hasMcp;
    default:
      return true;
  }
}
