/**
 * Cloudflare Access JWT verification for Worker-side auth gate.
 * Validates Cf-Access-Jwt-Assertion signature (RS256) and audience.
 */

interface AccessCertsResponse {
  keys?: Array<JsonWebKey & { kid?: string }>;
  public_certs?: string[];
}

let cachedCerts: { issuer: string; keys: Array<JsonWebKey & { kid?: string }>; fetchedAt: number } | null =
  null;
const CERT_CACHE_MS = 60 * 60 * 1000;

function base64UrlDecode(input: string): Uint8Array {
  const padded = input.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((input.length + 3) % 4);
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

function decodeJwtPart(part: string): Record<string, unknown> {
  const json = new TextDecoder().decode(base64UrlDecode(part));
  return JSON.parse(json) as Record<string, unknown>;
}

function audienceMatches(payload: Record<string, unknown>, expectedAud: string): boolean {
  const aud = payload.aud;
  if (typeof aud === "string") return aud === expectedAud;
  if (Array.isArray(aud)) return aud.some((v) => v === expectedAud);
  return false;
}

async function fetchAccessKeys(issuer: string): Promise<Array<JsonWebKey & { kid?: string }>> {
  const normalized = issuer.replace(/\/$/, "");
  if (cachedCerts && cachedCerts.issuer === normalized && Date.now() - cachedCerts.fetchedAt < CERT_CACHE_MS) {
    return cachedCerts.keys;
  }
  const res = await fetch(`${normalized}/cdn-cgi/access/certs`, { cf: { cacheTtl: 3600 } });
  if (!res.ok) throw new Error(`access certs fetch failed: ${res.status}`);
  const body = (await res.json()) as AccessCertsResponse;
  const keys = body.keys || [];
  cachedCerts = { issuer: normalized, keys, fetchedAt: Date.now() };
  return keys;
}

async function importVerifyKey(jwk: JsonWebKey & { kid?: string }): Promise<CryptoKey> {
  return crypto.subtle.importKey(
    "jwk",
    jwk,
    { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
    false,
    ["verify"],
  );
}

export async function verifyAccessJwt(token: string, expectedAud: string): Promise<boolean> {
  const parts = token.split(".");
  if (parts.length !== 3) return false;

  let payload: Record<string, unknown>;
  let header: Record<string, unknown>;
  try {
    header = decodeJwtPart(parts[0]);
    payload = decodeJwtPart(parts[1]);
  } catch {
    return false;
  }

  const iss = payload.iss;
  if (typeof iss !== "string" || !iss.startsWith("https://")) return false;
  if (!audienceMatches(payload, expectedAud)) return false;

  const exp = payload.exp;
  if (typeof exp !== "number" || exp <= Math.floor(Date.now() / 1000)) return false;

  const kid = header.kid;
  if (typeof kid !== "string") return false;

  let keys: Array<JsonWebKey & { kid?: string }>;
  try {
    keys = await fetchAccessKeys(iss);
  } catch {
    return false;
  }

  const jwk = keys.find((k) => k.kid === kid);
  if (!jwk) return false;

  let key: CryptoKey;
  try {
    key = await importVerifyKey(jwk);
  } catch {
    return false;
  }

  const data = new TextEncoder().encode(`${parts[0]}.${parts[1]}`);
  const signature = base64UrlDecode(parts[2]);
  try {
    return await crypto.subtle.verify("RSASSA-PKCS1-v1_5", key, signature, data);
  } catch {
    return false;
  }
}
