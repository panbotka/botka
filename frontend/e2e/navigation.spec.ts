import { test, expect } from '@playwright/test';
import { login, waitForApp } from './test-utils';

test.describe('Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    await waitForApp(page);
  });

  test('sidebar shows all nav links for admin user', async ({ page }) => {
    const sidebar = page.locator('aside');
    await expect(sidebar).toBeVisible();

    const labels = ['Dashboard', 'Chat', 'Tasks', 'Projects', 'Cost', 'Settings', 'Help'];
    for (const label of labels) {
      await expect(sidebar.locator(`a:has-text("${label}")`)).toBeVisible();
    }
  });

  test('clicking sidebar links navigates to correct pages', async ({ page }) => {
    const sidebar = page.locator('aside');

    // Navigate to Tasks
    await sidebar.locator('a:has-text("Tasks")').click();
    await expect(page).toHaveURL(/\/tasks/);

    // Navigate to Settings
    await sidebar.locator('a:has-text("Settings")').click();
    await expect(page).toHaveURL(/\/settings/);

    // Navigate to Help
    await sidebar.locator('a:has-text("Help")').click();
    await expect(page).toHaveURL(/\/help/);

    // Navigate to Dashboard
    await sidebar.locator('a:has-text("Dashboard")').click();
    await expect(page).toHaveURL('http://localhost:5110/');

    // Navigate to Chat (last, since it may auto-select a thread)
    await sidebar.locator('a:has-text("Chat")').click();
    await expect(page).toHaveURL(/\/chat/);
  });

  test('active nav link is highlighted', async ({ page }) => {
    const sidebar = page.locator('aside');

    // Navigate to Tasks page
    await sidebar.locator('a:has-text("Tasks")').click();
    await expect(page).toHaveURL(/\/tasks/);

    // The active link should have the active class (bg-zinc-200)
    const tasksLink = sidebar.locator('a:has-text("Tasks")');
    await expect(tasksLink).toHaveClass(/bg-zinc-200/);
  });

  test('each page loads correct content', async ({ page }) => {
    const sidebar = page.locator('aside');

    // Tasks page shows filter tabs
    await sidebar.locator('a:has-text("Tasks")').click();
    await expect(page).toHaveURL(/\/tasks/);
    await expect(page.locator('button:has-text("All")').first()).toBeVisible();
    await expect(page.locator('button:has-text("Pending")').first()).toBeVisible();

    // Settings page shows theme controls
    await sidebar.locator('a:has-text("Settings")').click();
    await expect(page).toHaveURL(/\/settings/);
    await expect(page.locator('text=Theme')).toBeVisible();

    // Help page shows content
    await sidebar.locator('a:has-text("Help")').click();
    await expect(page).toHaveURL(/\/help/);
    await expect(page.locator('main')).toBeVisible();
  });
});
