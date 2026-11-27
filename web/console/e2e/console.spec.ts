import { expect, test, type APIRequestContext } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";

const API = process.env.API_URL ?? "http://localhost:8080";
const OIDC = process.env.DEVOIDC_URL ?? "http://localhost:9000";
const CLOUD = process.env.PHASE === "cloud";
const SHOTS = process.env.SHOT_DIR ?? "test-results/screenshots";
const name = (local: string, cloud: string) => (CLOUD ? cloud : local);

async function token(request: APIRequestContext): Promise<string> {
  if (process.env.TOKEN) return process.env.TOKEN;
  const r = await request.post(`${OIDC}/token`, { data: { operator: "demo" } });
  return (await r.json()).access_token;
}

test("book a pass, watch live telemetry, SLA and billing update", async ({ page, request }) => {
  fs.mkdirSync(SHOTS, { recursive: true });
  const shot = (file: string) => page.screenshot({ path: path.join(SHOTS, `${file}.png`), fullPage: false });
  const tok = await token(request);
  const auth = { Authorization: `Bearer ${tok}` };

  // find the first unbooked Tokyo pass of the ISS in a fixed window and jump simulated time to 8 min before AOS
  const sats = await (await request.get(`${API}/satellites`, { headers: auth })).json();
  const iss = sats.find((s: { name: string }) => s.name.startsWith("ISS"));
  expect(iss, "seed satellite ISS").toBeTruthy();
  const from = "2026-11-27T09:00:00Z";
  const to = "2026-11-28T09:00:00Z";
  const passes = await (await request.get(`${API}/passes?satellite=${iss.id}&station=tokyo&from=${from}&to=${to}`, { headers: auth })).json();
  const bookings = await (await request.get(`${API}/bookings`, { headers: auth })).json();
  const taken = new Set(bookings.map((b: { pass_id: string }) => b.pass_id));
  const pass = passes.find((p: { id: string }) => !taken.has(p.id));
  expect(pass, "an unbooked Tokyo pass").toBeTruthy();
  await request.put(`${API}/sim/clock`, { headers: auth, data: { sim_time: new Date(new Date(pass.aos).getTime() - 8 * 60e3).toISOString(), scale: 20 } });

  // sign in: through the hosted-login redirect locally, with an injected token in the cloud
  if (CLOUD) {
    await page.addInitScript((t) => localStorage.setItem("gs_token", t), tok);
    await page.goto("/");
  } else {
    await page.goto("/");
    await page.getByTestId("login").click();
  }
  await expect(page.getByTestId("stations-map")).toBeVisible();
  await expect(page.getByTestId("stations-table").getByText("tokyo")).toBeVisible();
  await expect(page.getByTestId("stations-table").getByText("idle").first()).toBeVisible();
  if (!CLOUD) await shot("01-stations-map");

  await page.getByRole("link", { name: "Passes" }).click();
  await expect(page.getByTestId("pass-tokyo").first()).toBeVisible();
  await shot(name("02-passes-timeline", "01-cf-passes"));

  await page.getByTestId("pass-tokyo").first().click({ force: true });
  await page.getByTestId("book").click();
  await expect(page.getByTestId("message")).toContainText("Booked");
  await expect(page.getByTestId("booking-status").first()).toBeVisible();
  if (!CLOUD) await shot("03-booking");

  await page.getByRole("link", { name: "Live telemetry" }).click();
  await page.getByTestId("station-tokyo").click();
  await expect(page.getByTestId("tile-lock")).toContainText("LOCKED");
  await expect(async () => {
    expect(Number(await page.getByTestId("chart-snr").getAttribute("data-points"))).toBeGreaterThanOrEqual(16);
  }).toPass({ timeout: 60_000 });
  await shot(name("04-live-telemetry", "02-cf-live-telemetry"));

  await page.getByRole("link", { name: "SLA" }).click();
  await expect(page.getByTestId("sla-total")).toContainText(/[1-9]/, { timeout: 120_000 });
  await shot(name("05-sla", "03-cf-sla"));

  await page.getByRole("link", { name: "Billing" }).click();
  await expect(page.getByTestId("billing-table").getByText("tokyo")).toBeVisible();
  await shot(name("06-billing", "04-cf-billing"));
});
