import { test, expect } from '@playwright/test';
import { login, logout, waitForAlpine, resetTestDatabase, seedScenario } from '../../fixtures';

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
		await waitForAlpine(page);

		// Ensure authorized users have at least one Logs navigation entry
		const logsNavLink = page.locator('a[href="/logs"]').filter({ hasText: /logs/i });
		const linkCount = await logsNavLink.count();
		expect(linkCount).toBeGreaterThan(0);

		const response = await page.goto('/logs', { waitUntil: 'domcontentloaded' });
		if (response) {
			expect(response.status()).toBeLessThan(400);
		}
		await expect(page).toHaveURL(/\/logs/);
		await expect(page.locator('[data-policy-inspector]')).toBeVisible();
		await expect(page.getByRole('heading', { level: 1 })).toContainText(/Logs/i);
		await expect(page.getByRole('button', { name: /Authentication Logs/i })).toBeVisible();
		await expect(page.getByRole('button', { name: /Action Logs/i })).toBeVisible();
	});

	test('shows logs page in restricted mode for user without logging permissions', async ({ page }) => {
		await login(page, 'nohrm@example.com', 'TestPass123!');

		const response = await page.goto('/logs', { waitUntil: 'domcontentloaded' });
		if (response) {
			expect(response.status()).toBeLessThan(400);
		}
		await expect(page.getByRole('heading', { level: 1 })).toContainText(/Logs/i);
		await expect(page.locator('.pointer-events-none.select-none')).toBeVisible();
		await expect(page.locator('[data-policy-inspector]')).toHaveCount(0);
		const requestForm = page.locator('form[hx-post="/core/api/authz/requests"]');
		if (await requestForm.count()) {
			await expect(requestForm.first()).toBeVisible();
		}

		const apiResponse = await page.request.get('/logs', {
			headers: { Accept: 'application/json' },
		});
		expect(apiResponse.status()).toBe(403);
		const body = await apiResponse.json();
		expect(body.object).toBe('logging.logs');
		expect(body.action).toBe('view');
		expect(typeof body.subject).toBe('string');
		expect(body.subject).toMatch(/tenant:/);
		expect(body.domain).toBe('logging');
		expect(Array.isArray(body.missing_policies)).toBeTruthy();
		expect(body.missing_policies.length).toBeGreaterThan(0);
		expect(body.debug_url).toContain('/core/api/authz/debug');
		expect(body.base_revision).toBeTruthy();

		const apiResponseWithRequestID = await page.request.get('/logs', {
			headers: { Accept: 'application/json', 'X-Request-ID': 'e2e-logs-req' },
		});
		expect(apiResponseWithRequestID.status()).toBe(403);
		const bodyWithRequestID = await apiResponseWithRequestID.json();
		expect(bodyWithRequestID.request_id).toBe('e2e-logs-req');
		expect(bodyWithRequestID.base_revision).toBeTruthy();
	});
});
