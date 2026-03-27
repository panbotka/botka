import { test, expect } from '@playwright/test';
import { login, waitForApp } from './test-utils';

test.describe('Settings Flow', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    await waitForApp(page);
    // Navigate to settings
    await page.locator('aside a:has-text("Settings")').click();
    await expect(page).toHaveURL(/\/settings/);
  });

  test('settings page shows General tab by default', async ({ page }) => {
    // Theme and Font Size controls should be visible on General tab
    await expect(page.locator('text=Theme')).toBeVisible();
    await expect(page.locator('text=Font Size')).toBeVisible();
  });

  test('settings page shows all tabs', async ({ page }) => {
    const tabs = ['General', 'Security', 'Users', 'Task Runner', 'Personas', 'Tags', 'Memories', 'Voice'];
    for (const tab of tabs) {
      await expect(page.locator(`button:has-text("${tab}")`).first()).toBeVisible();
    }
  });

  test('can change theme', async ({ page }) => {
    // Click "Dark" theme button
    await page.locator('button:has-text("Dark")').first().click();
    await page.waitForTimeout(300);

    // The "Dark" button should now have the active styling (bg-zinc-900 or dark variant)
    const darkButton = page.locator('button:has-text("Dark")').first();
    await expect(darkButton).toHaveClass(/bg-zinc-/);

    // Switch back to "Light"
    await page.locator('button:has-text("Light")').first().click();
    await page.waitForTimeout(300);

    const lightButton = page.locator('button:has-text("Light")').first();
    await expect(lightButton).toHaveClass(/bg-zinc-900|bg-zinc-200/);
  });

  test('can change font size', async ({ page }) => {
    // Click "Large" font size
    await page.locator('button:has-text("large")').click();
    await page.waitForTimeout(300);

    // Verify the button is active
    const largeButton = page.locator('button:has-text("large")');
    await expect(largeButton).toHaveClass(/bg-zinc-900|bg-zinc-200/);

    // Click "Small" font size
    await page.locator('button:has-text("small")').click();
    await page.waitForTimeout(300);

    const smallButton = page.locator('button:has-text("small")');
    await expect(smallButton).toHaveClass(/bg-zinc-900|bg-zinc-200/);

    // Restore to "Medium"
    await page.locator('button:has-text("medium")').click();
    await page.waitForTimeout(300);
  });

  test('can switch between settings tabs', async ({ page }) => {
    // Click "Personas" tab
    await page.locator('button:has-text("Personas")').first().click();
    await page.waitForTimeout(500);

    // Should show persona-related content or empty state
    const main = page.locator('main');
    await expect(main).toBeVisible();

    // Click "Tags" tab
    await page.locator('button:has-text("Tags")').first().click();
    await page.waitForTimeout(500);
    await expect(main).toBeVisible();

    // Go back to General
    await page.locator('button:has-text("General")').first().click();
    await expect(page.locator('text=Theme')).toBeVisible();
  });

  test('theme persists after page reload', async ({ page }) => {
    // Switch to Dark theme
    await page.locator('button:has-text("Dark")').first().click();
    await page.waitForTimeout(300);

    // Reload the page
    await page.reload();
    await page.waitForLoadState('networkidle');

    // Navigate back to settings
    await page.locator('aside a:has-text("Settings")').click();
    await page.waitForTimeout(500);

    // The "Dark" button should still be active
    const darkButton = page.locator('button:has-text("Dark")').first();
    await expect(darkButton).toHaveClass(/bg-zinc-/);

    // Reset to Light for clean state
    await page.locator('button:has-text("Light")').first().click();
  });
});
