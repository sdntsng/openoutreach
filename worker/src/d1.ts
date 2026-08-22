export interface D1Env {
  DB?: D1Database;
  INTERNAL_CONTAINER_TOKEN?: string;
}

function bindStmt(db: D1Database, sql: string, params?: unknown[]): D1PreparedStatement {
  const stmt = db.prepare(sql);
  if (params && params.length > 0) {
    return stmt.bind(...params);
  }
  return stmt;
}

async function runQuery(db: D1Database, sql: string, params?: unknown[]) {
  const all = await bindStmt(db, sql, params).all();
  const results = (all.results || []) as Record<string, unknown>[];
  const columns = results[0] ? Object.keys(results[0]) : [];
  return {
    columns,
    rows: results.map((row) => columns.map((c) => row[c])),
    meta: {
      changes: all.meta?.changes ?? 0,
      last_row_id: all.meta?.last_row_id ?? 0,
    },
  };
}

async function runExec(db: D1Database, sql: string, params?: unknown[]) {
  if (params && params.length > 0) {
    const result = await bindStmt(db, sql, params).run();
    return {
      meta: {
        changes: result.meta?.changes ?? 0,
        last_row_id: result.meta?.last_row_id ?? 0,
      },
    };
  }
  const statements = sql
    .split(";")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
  let changes = 0;
  let lastRowId = 0;
  for (const stmt of statements) {
    const result = await db.prepare(stmt).run();
    changes += result.meta?.changes ?? 0;
    lastRowId = result.meta?.last_row_id ?? lastRowId;
  }
  return {
    meta: {
      changes,
      last_row_id: lastRowId,
    },
  };
}

export async function handleD1(request: Request, env: D1Env): Promise<Response> {
  if (request.method !== "POST") {
    return json({ error: "method not allowed" }, 405);
  }
  if (!env.DB) {
    return json({ error: "D1 binding DB is not configured" }, 501);
  }
  if (env.INTERNAL_CONTAINER_TOKEN) {
    if (request.headers.get("X-Internal-Token") !== env.INTERNAL_CONTAINER_TOKEN) {
      return json({ error: "unauthorized" }, 401);
    }
  }

  let body: { sql?: string; params?: unknown[]; mode?: string };
  try {
    body = (await request.json()) as { sql?: string; params?: unknown[]; mode?: string };
  } catch {
    return json({ error: "invalid json" }, 400);
  }

  try {
    if (body.mode === "query" && body.sql) {
      return json(await runQuery(env.DB, body.sql, body.params));
    }
    if (body.mode === "exec" && body.sql) {
      return json(await runExec(env.DB, body.sql, body.params));
    }
    return json({ error: "sql and mode required" }, 400);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return json({ error: message }, 400);
  }
}

function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
