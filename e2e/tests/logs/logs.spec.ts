import { test, expect } from '@playwright/test';
import { login, logout } from '../../fixtures/auth';
import { resetTestDatabase, seedScenario } from '../../fixtures/test-data';

test.describe.configure({ mode: 'serial' });

test.describe('logging authz gating', () => {
	test.beforeAll(async ({ request }) => {
		await resetTestDatabase(request, { reseedMinimal: false });
		await seedScenario(request, 'comprehensive');
	});

	test.beforeEach(async ({ page }) => {
		await page.setViewportSize({ width: 1280, height: 720 });
	});

	test.afterEach(async ({ page }) => {
		await logout(page);
	});

	test('allows superadmin to view logs page', async ({ page }) => {
		await login(page, 'test@gmail.com', 'TestPass123!');

		const response = await page.goto('/logs', { waitUntil: 'domcontentloaded' });
		if (response) {
			expect(response.status()).toBeLessThan(400);
		}
		await expect(page).toHaveURL(/\/logs/);
		await expect(page.getByRole('heading', { level: 1 })).toContainText(/Logs/i);
	});

	test('blocks logs page for user without logging permissions', async ({ page }) => {
		await login(page, 'nohrm@example.com', 'TestPass123!');

		const response = await page.goto('/logs', { waitUntil: 'domcontentloaded' });
		if (response) {
			expect([401, 403]).toContain(response.status());
		}
		await expect(page.getByText(/Permission required/i)).toBeVisible();
		await expect(page.getByRole('link', { name: /Request access/i })).toBeVisible();
	});
});
