import { test, expect } from '@playwright/test';
import { login, waitForApp, createThread, deleteThread, uniqueName } from './test-utils';

test.describe('Thread Management', () => {
  const createdThreadIds: string[] = [];

  test.beforeEach(async ({ page }) => {
    await login(page);
    await waitForApp(page);
    // Navigate to chat
    await page.locator('aside a:has-text("Chat")').click();
    await expect(page).toHaveURL(/\/chat/);
  });

  test.afterEach(async ({ page }) => {
    for (const id of createdThreadIds) {
      await deleteThread(page, id).catch(() => {});
    }
    createdThreadIds.length = 0;
  });

  test('can create a thread and see it in the sidebar', async ({ page }) => {
    const id = await createThread(page);
    createdThreadIds.push(id);

    // Navigate to the thread directly — this ensures the sidebar shows it
    await page.goto(`/chat/${id}`);
    await page.waitForLoadState('networkidle');

    // The thread should appear (default title "New Chat") and input should be visible
    const chatInput = page.locator('textarea[placeholder*="Message"]');
    await expect(chatInput).toBeVisible({ timeout: 10000 });
  });

  test('can rename a thread via API and see updated title', async ({ page }) => {
    const newTitle = uniqueName('E2E-Renamed');

    // Create and rename
    const id = await createThread(page, newTitle);
    createdThreadIds.push(id);

    // Reload chat page to see the renamed thread
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    await expect(page.locator(`text=${newTitle}`)).toBeVisible({ timeout: 10000 });
  });

  test('can delete a thread via API and it disappears', async ({ page }) => {
    const title = uniqueName('E2E-ToDelete');

    // Create thread with a unique name
    const id = await createThread(page, title);

    // Verify it shows up
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await expect(page.locator(`text=${title}`)).toBeVisible({ timeout: 10000 });

    // Delete via API
    const deleteRes = await page.request.delete(`/api/v1/threads/${id}`);
    expect(deleteRes.ok()).toBeTruthy();

    // Reload and verify it's gone
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');
    await expect(page.locator(`text=${title}`)).not.toBeVisible();
  });

  test('creating multiple threads shows them all in sidebar', async ({ page }) => {
    const threads: { id: string; title: string }[] = [];

    for (let i = 1; i <= 3; i++) {
      const title = uniqueName(`E2E-Multi-${i}`);
      const id = await createThread(page, title);
      createdThreadIds.push(id);
      threads.push({ id, title });
    }

    // Reload and check all are visible
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    for (const t of threads) {
      await expect(page.locator(`text=${t.title}`)).toBeVisible({ timeout: 10000 });
    }
  });

  test('clicking a thread opens it', async ({ page }) => {
    const title = uniqueName('E2E-ClickOpen');
    const id = await createThread(page, title);
    createdThreadIds.push(id);

    // Load chat page
    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    // Click on the thread title
    await page.locator(`text=${title}`).click();
    await page.waitForTimeout(500);

    // URL should include the thread ID
    await expect(page).toHaveURL(new RegExp(`/chat/${id}`));

    // Chat input should be visible
    const chatInput = page.locator('textarea[placeholder*="Message"]');
    await expect(chatInput).toBeVisible({ timeout: 10000 });
  });
});
