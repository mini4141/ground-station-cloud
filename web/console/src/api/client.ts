import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

export type Station = components["schemas"]["Station"];
export type Satellite = components["schemas"]["Satellite"];
export type Pass = components["schemas"]["Pass"];
export type Booking = components["schemas"]["Booking"];
export type LiveState = components["schemas"]["LiveState"];
export type SlaRow = components["schemas"]["SlaRow"];
export type BillingRow = components["schemas"]["BillingRow"];
export type SimClock = components["schemas"]["SimClock"];

export interface Config {
  apiUrl: string;
  authMode: "dev" | "cognito";
  devOidcUrl?: string;
  cognitoDomain?: string;
  cognitoClientId?: string;
}

const TOKEN_KEY = "gs_token";

export async function loadConfig(): Promise<Config> {
  const r = await fetch("/config.json", { cache: "no-store" });
  return r.json();
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

/** Picks up the token the hosted login (Cognito or dev OIDC) puts in the URL fragment. */
export function captureTokenFromHash(): void {
  const params = new URLSearchParams(location.hash.replace(/^#/, ""));
  const t = params.get("access_token");
  if (t) {
    localStorage.setItem(TOKEN_KEY, t);
    history.replaceState(null, "", location.pathname);
  }
}

export function logout(): void {
  localStorage.removeItem(TOKEN_KEY);
  location.reload();
}

export function loginUrl(cfg: Config): string {
  const redirect = encodeURIComponent(location.origin + "/");
  if (cfg.authMode === "cognito") {
    return `https://${cfg.cognitoDomain}/oauth2/authorize?client_id=${cfg.cognitoClientId}&response_type=token&scope=openid+email&redirect_uri=${redirect}`;
  }
  return `${cfg.devOidcUrl}/login?operator=demo&redirect_uri=${redirect}`;
}

export function makeClient(cfg: Config) {
  const c = createClient<paths>({ baseUrl: cfg.apiUrl });
  c.use({
    onRequest({ request }) {
      const t = getToken();
      if (t) request.headers.set("Authorization", `Bearer ${t}`);
      return request;
    },
  });
  return c;
}

export type Api = ReturnType<typeof makeClient>;
