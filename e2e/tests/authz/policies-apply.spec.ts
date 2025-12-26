import { test, expect } from '@playwright/test';
import { login, logout, waitForAlpine, resetTestDatabase, seedScenario } from '../../fixtures';

test.describe.configure({ mode: 'serial' });

test.describe('authz policies apply', () => {
	test.beforeAll(async ({ request }) => {
		await resetTestDatabase(request, { reseedMinimal: false });
		await seedScenario(request, 'comprehensive');
	});

	test.afterEach(async ({ page }) => {
		await logout(page);
	});

	test('can apply and rollback direct policy changes', async ({ page }) => {
		await login(page, 'test@gmail.com', 'TestPass123!');
		await waitForAlpine(page);

		await page.goto('/users?Search=noperson@example.com', { waitUntil: 'domcontentloaded' });
		await expect(page.locator('#users-table-body')).toBeVisible();

		const userEditLink = page.locator('#users-table-body a[href^="/users/"]').first();
		await expect(userEditLink).toBeVisible();
		await userEditLink.click();
		await expect(page).toHaveURL(/\/users\/[0-9]+$/);
		const userURL = page.url();

			await page.locator('button', { hasText: /permissions/i }).first().click();
			await expect(page.locator('#user-policy-board')).toBeVisible();

		const addMenuDetails = page.locator('details:has([data-testid="authz-user-stage-menu"])').first();
		await addMenuDetails.evaluate(el => {
			(el as unknown as { open: boolean }).open = true;
		});
		await page.getByTestId('authz-user-stage-open-direct').click();

			const stageDialog = page.locator('#stage-policy-direct dialog');
			await expect(stageDialog).toBeVisible();

		await stageDialog.locator('input[name="object"]').fill('logging.logs');
		await stageDialog.locator('input[name="action"]').fill('view');

			await page.getByTestId('authz-user-stage-save-direct').click();
			await expect(page.locator('#authz-workspace')).toHaveCount(1, { timeout: 15_000 });
			await expect(page.locator('#user-policy-board')).toBeVisible({ timeout: 15_000 });
			await page.waitForFunction(() => {
				const el = document.getElementById('user-policy-board');
				return !!el && !el.classList.contains('htmx-request');
			});

			await page.locator('#authz-workspace').evaluate(form => (form as HTMLFormElement).requestSubmit());
			await expect(page.locator('#authz-workspace')).toHaveCount(0, { timeout: 15_000 });

		const appliedScreenshot = test.info().outputPath('authz-apply-admin.png');
		await page.screenshot({ path: appliedScreenshot, fullPage: true });
		test.info().attach('authz-apply-admin.png', { path: appliedScreenshot, contentType: 'image/png' });

		await logout(page);
		await login(page, 'noperson@example.com', 'TestPass123!');
		await page.goto('/logs', { waitUntil: 'domcontentloaded' });
		await expect(page.getByRole('heading', { level: 1 })).toContainText(/Logs/i);
		await expect(page.locator('[data-policy-inspector]')).toBeVisible();

		const allowedScreenshot = test.info().outputPath('authz-apply-allowed-user.png');
		await page.screenshot({ path: allowedScreenshot, fullPage: true });
		test.info().attach('authz-apply-allowed-user.png', { path: allowedScreenshot, contentType: 'image/png' });

		await logout(page);
		await login(page, 'test@gmail.com', 'TestPass123!');
		await waitForAlpine(page);

			await page.goto(userURL, { waitUntil: 'domcontentloaded' });
			await page.locator('button', { hasText: /permissions/i }).first().click();
			await expect(page.locator('#user-policy-board')).toBeVisible();

			const directColumn = page.locator('#user-policy-direct');
			const ruleRow = directColumn.locator('tr', { hasText: 'logging.logs' }).filter({ hasText: 'view' }).first();
		await expect(ruleRow).toBeVisible();

			await ruleRow.getByRole('button', { name: /delete/i }).click();
			await expect(page.locator('#authz-workspace')).toHaveCount(1, { timeout: 15_000 });
			await expect(page.locator('#user-policy-board')).toBeVisible({ timeout: 15_000 });
			await page.waitForFunction(() => {
				const el = document.getElementById('user-policy-board');
				return !!el && !el.classList.contains('htmx-request');
			});

			await page.locator('#authz-workspace').evaluate(form => (form as HTMLFormElement).requestSubmit());
			await expect(page.locator('#authz-workspace')).toHaveCount(0, { timeout: 15_000 });
	});
});
