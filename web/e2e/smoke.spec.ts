import { test, expect } from "@playwright/test";

test.describe("smoke", () => {
  test("unauthenticated user is redirected to /login", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/login/);
  });

  test("login page renders with sign-in button", async ({ page }) => {
    await page.goto("/login");
    // Match both EN ("Sign in with SSO") and DE ("Mit SSO anmelden")
    await expect(
      page.getByRole("button", { name: /sign in|anmelden/i }),
    ).toBeVisible();
  });

  test("health endpoint returns ok", async ({ request }) => {
    const response = await request.get("/api/health");
    expect(response.ok()).toBeTruthy();
    expect(await response.json()).toEqual(
      expect.objectContaining({ status: "ok" }),
    );
  });

  test("security headers are present", async ({ request }) => {
    const response = await request.get("/login");
    const headers = response.headers();
    expect(headers["content-security-policy"]).toContain("default-src 'self'");
    expect(headers["strict-transport-security"]).toContain("max-age=63072000");
    expect(headers["x-frame-options"]).toBe("DENY");
    expect(headers["x-content-type-options"]).toBe("nosniff");
  });
});
