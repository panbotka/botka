import { test, expect } from '@playwright/test';
import { login, waitForApp, createThread, deleteThread } from './test-utils';

test.describe('Chat Flow', () => {
  let threadId: string;

  test.beforeEach(async ({ page }) => {
    await login(page);
    await waitForApp(page);
    // Navigate to chat
    await page.locator('aside a:has-text("Chat")').click();
    await expect(page).toHaveURL(/\/chat/);
  });

  test.afterEach(async ({ page }) => {
    if (threadId) {
      await deleteThread(page, threadId);
      threadId = '';
    }
  });

  test('chat page loads with thread sidebar', async ({ page }) => {
    const chatArea = page.locator('main');
    await expect(chatArea).toBeVisible();
  });

  test('can create a new thread and see chat input', async ({ page }) => {
    threadId = await createThread(page);

    await page.goto(`/chat/${threadId}`);
    await page.waitForLoadState('networkidle');

    const chatInput = page.locator('textarea[placeholder*="Message"]');
    await expect(chatInput).toBeVisible({ timeout: 10000 });
  });

  test('can type in chat input', async ({ page }) => {
    threadId = await createThread(page);

    await page.goto(`/chat/${threadId}`);
    await page.waitForLoadState('networkidle');

    const chatInput = page.locator('textarea[placeholder*="Message"]');
    await expect(chatInput).toBeVisible({ timeout: 10000 });

    await chatInput.fill('Hello, this is an E2E test message');
    await expect(chatInput).toHaveValue('Hello, this is an E2E test message');
  });

  test('thread appears in sidebar after creation', async ({ page }) => {
    // Threads are created as "New Chat" by default
    threadId = await createThread(page);

    await page.goto('/chat');
    await page.waitForLoadState('networkidle');

    // Should see "New Chat" in the sidebar (the default title)
    await expect(page.locator('text=New Chat').first()).toBeVisible({ timeout: 10000 });
  });
});
