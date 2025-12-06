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

	test('allows superadmin to view logs page and tabs', async ({ page }) => {
		await login(page, 'test@gmail.com', 'TestPass123!');

		// Prefer visible expanded link to avoid grabbing the collapsed (hidden) variant
		const logsNavLink = page.locator('a[href="/logs"]').filter({ hasText: /logs/i }).first();
		await expect(logsNavLink).toBeVisible();

		const response = await page.goto('/logs', { waitUntil: 'domcontentloaded' });
		if (response) {
			expect(response.status()).toBeLessThan(400);
		}
		await expect(page).toHaveURL(/\/logs/);
		await expect(page.getByRole('heading', { level: 1 })).toContainText(/Logs/i);
		await expect(page.getByRole('button', { name: /Authentication Logs/i })).toBeVisible();
		await expect(page.getByRole('button', { name: /Action Logs/i })).toBeVisible();
	});

	test('blocks logs page for user without logging permissions', async ({ page }) => {
		await login(page, 'nohrm@example.com', 'TestPass123!');

		const logsNavLink = page.locator('a[href="/logs"]').filter({ hasText: /logs/i }).first();
		if (await logsNavLink.count()) {
			await logsNavLink.scrollIntoViewIfNeeded();
		}

		const response = await page.goto('/logs', { waitUntil: 'domcontentloaded' });
		if (response) {
			expect([401, 403]).toContain(response.status());
		}
		await expect(page.getByText(/Permission required/i)).toBeVisible();
		await expect(page.getByRole('link', { name: /Request access/i })).toBeVisible();

		const apiResponse = await page.request.get('/logs', {
			headers: { Accept: 'application/json' },
		});
		expect(apiResponse.status()).toBe(403);
		const body = await apiResponse.json();
		expect(body.object).toBe('logging.logs');
		expect(body.action).toBe('view');
		expect(typeof body.subject).toBe('string');
		expect(body.subject).toMatch(/tenant:/);
		expect(body.domain).toBeTruthy();
		expect(Array.isArray(body.missing_policies)).toBeTruthy();
		expect(body.missing_policies.length).toBeGreaterThan(0);
		expect(body.debug_url).toContain('/core/api/authz/debug');
	});
});
