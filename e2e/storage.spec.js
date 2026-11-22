const { test, expect } = require('./fixtures');

// The storage section of Settings: the sizes come from the backend, and the cleanup
// removes only what the preview listed.
test('the storage section shows the sizes and previews the cleanup', async ({ window }) => {
  await window.locator('#tab-settings').click();
  await expect(window.locator('#settings-pane')).toBeVisible();

  await expect(window.locator('#db-size')).toContainText('KB');
  await expect(window.locator('#total-size')).toContainText('KB');
  await expect(window.locator('#data-dir')).toContainText('fistbump-e2e-');

  // Nothing is chosen yet, so the preview asks for a choice.
  await window.locator('#cleanup-preview').click();
  await expect(window.locator('#cleanup-status')).toContainText('Choose what to remove');

  // With the cache chosen, the preview reports what would go and removes nothing.
  await window.locator('#cleanup-cache').check();
  await window.locator('#cleanup-preview').click();
  await expect(window.locator('#cleanup-status')).toContainText('Would remove');

  // The cleanup empties the cache and says so.
  await window.locator('#cleanup-run').click();
  await window.locator('#cleanup-run').click();
  await expect(window.locator('#cleanup-status')).toContainText('Removed');
});
