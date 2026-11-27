import { expect, test } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";

// Local only: screenshot a traced API request (GET /passes with its SQL spans) in the Jaeger UI.
test.skip(process.env.PHASE === "cloud", "local stack only");
test("jaeger shows an API trace", async ({ page, request }) => {
  const shots = process.env.SHOT_DIR ?? "test-results/screenshots";
  fs.mkdirSync(shots, { recursive: true });
  const jaeger = process.env.JAEGER_URL ?? "http://localhost:16686";
  let id = "";
  await expect(async () => {
    const r = await request.get(`${jaeger}/api/traces?service=api&operation=${encodeURIComponent("GET /passes")}&limit=20&lookback=1h`);
    const traces = (await r.json()).data as { traceID: string; spans: unknown[] }[];
    const best = traces?.sort((a, b) => b.spans.length - a.spans.length)[0];
    expect(best).toBeTruthy();
    id = best.traceID;
  }).toPass({ timeout: 30_000 });
  await page.goto(`${jaeger}/trace/${id}`);
  await expect(page.getByText("api").first()).toBeVisible();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: path.join(shots, "07-jaeger-trace.png") });
});
