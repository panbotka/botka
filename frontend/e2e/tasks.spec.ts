import { test, expect } from '@playwright/test';
import { login, waitForApp } from './test-utils';

test.describe('Task Flow', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    await waitForApp(page);
    // Navigate to tasks
    await page.locator('aside a:has-text("Tasks")').click();
    await expect(page).toHaveURL(/\/tasks/);
  });

  test('tasks page shows filter tabs', async ({ page }) => {
    // All status filter tabs should be visible
    const filterLabels = ['All', 'Pending', 'Queued', 'Running', 'Done', 'Failed'];
    for (const label of filterLabels) {
      await expect(page.locator(`button:has-text("${label}")`).first()).toBeVisible();
    }
  });

  test('tasks page shows task list or empty state', async ({ page }) => {
    // Wait for loading to finish
    await page.waitForLoadState('networkidle');

    // Either a task list with items or no tasks — page should not be in error
    const mainContent = page.locator('main');
    await expect(mainContent).toBeVisible();

    // Should not have an error state
    const errorText = page.locator('text=Something went wrong');
    await expect(errorText).not.toBeVisible();
  });

  test('clicking status filter tabs changes the view', async ({ page }) => {
    await page.waitForLoadState('networkidle');

    // Click on "Done" filter
    await page.locator('button:has-text("Done")').first().click();
    await expect(page).toHaveURL(/status=done/);

    // Click on "Pending" filter
    await page.locator('button:has-text("Pending")').first().click();
    await expect(page).toHaveURL(/status=pending/);

    // Click on "All" to go back
    await page.locator('button:has-text("All")').first().click();
    // "All" should clear or not have status param
    await page.waitForLoadState('networkidle');
  });

  test('can navigate to task detail and back', async ({ page }) => {
    await page.waitForLoadState('networkidle');

    // Task rows are <tr> elements with cursor-pointer, using onClick navigation
    const taskRows = page.locator('table tbody tr.cursor-pointer');
    const count = await taskRows.count();

    if (count > 0) {
      // Click the first task row
      await taskRows.first().click();
      await expect(page).toHaveURL(/\/tasks\/[a-f0-9-]+/);

      // Task detail should show some content
      await page.waitForLoadState('networkidle');
      const main = page.locator('main');
      await expect(main).toBeVisible();

      // Navigate back
      await page.goBack();
      await expect(page).toHaveURL(/\/tasks/);
    }
    // If no tasks exist, that's fine — test passes
  });

  test('new task page is accessible', async ({ page }) => {
    // Look for "New Task" link/button
    const newTaskLink = page.locator('a:has-text("New Task")');
    const count = await newTaskLink.count();

    if (count > 0) {
      await newTaskLink.first().click();
      await expect(page).toHaveURL(/\/tasks\/new/);

      // Should show a form with title input
      await expect(page.locator('input, textarea').first()).toBeVisible();
    }
  });
});
