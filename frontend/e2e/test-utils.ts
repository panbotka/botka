import { type Page, expect } from '@playwright/test';

// ── Credentials ──
// Use env vars or fall back to the E2E test user.
// Create the test user with: make test-e2e-setup
export const USERNAME = process.env.E2E_USERNAME ?? 'e2e-test';
export const PASSWORD = process.env.E2E_PASSWORD ?? 'e2e-test-password';

// ── Common selectors ──
export const SEL = {
  bottomNav: 'nav.fixed.bottom-0',
  chatInput: 'textarea[placeholder*="Message"]',
  threadSidebar: '[class*="thread"]',
  sendButton: 'button:has(svg)',
  loginForm: 'form',
  usernameInput: '#username',
  passwordInput: '#password',
  signInButton: 'button:has-text("Sign in")',
} as const;

// ── Helpers ──

/**
 * Log in via the login page and wait for the app to load.
 */
export async function login(page: Page): Promise<void> {
  await page.goto('/login');
  await page.fill(SEL.usernameInput, USERNAME);
  await page.fill(SEL.passwordInput, PASSWORD);
  await page.click(SEL.signInButton);
  // Wait for redirect away from /login.
  await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
}

/**
 * Wait for the app shell to be fully loaded (bottom nav visible).
 */
export async function waitForApp(page: Page): Promise<void> {
  await page.waitForSelector('nav', { timeout: 10000 });
}

/**
 * Navigate to a tab by clicking the bottom nav button with the given label.
 */
export async function navigateTo(page: Page, tab: string): Promise<void> {
  await page.click(`nav button:has-text("${tab}")`);
  await page.waitForTimeout(500);
}

/**
 * Create a new chat thread via the API. If a name is given, rename it.
 * Returns the thread ID.
 */
export async function createThread(page: Page, name?: string): Promise<string> {
  const response = await page.request.post('/api/v1/threads', {
    data: {},
  });
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  const id = String(body.data.id);

  if (name) {
    const renameRes = await page.request.put(`/api/v1/threads/${id}`, {
      data: { title: name },
    });
    expect(renameRes.ok()).toBeTruthy();
  }

  return id;
}

/**
 * Delete a thread via the API.
 */
export async function deleteThread(page: Page, threadId: string): Promise<void> {
  await page.request.delete(`/api/v1/threads/${threadId}`);
}

/**
 * Generate a unique name for test resources.
 */
export function uniqueName(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
}
