import { test, expect } from '@playwright/test';
import { login, logout } from '../../fixtures/auth';
import { resetTestDatabase, seedScenario } from '../../fixtures/test-data';

test.describe('persons list page and authz gating', () => {
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

	test('displays persons list page', async ({ page }) => {
		await login(page, 'test@gmail.com', 'TestPass123!');

		await page.goto('/person/persons');
		await expect(page).toHaveURL(/\/person\/persons$/);

		// Check page title and main elements
		await expect(page.locator('h1')).toContainText('Persons');
		await expect(page.locator('a[href="/person/persons/new"]')).toBeVisible();

		// Check search and filter form
		await expect(page.locator('form input[name="q"]')).toBeVisible();
	});

	test('blocks persons list for user without person permissions', async ({ page }) => {
		await login(page, 'noperson@example.com', 'TestPass123!');

		const response = await page.goto('/person/persons');
		if (response) {
			expect([401, 403]).toContain(response.status());
		}
		await expect(page.getByRole('heading', { name: /Permission required/i, level: 2 })).toBeVisible();
		const container = page.locator('section[data-authz-container]');
		await expect(container).toBeVisible();
		const uiDomain = await container.getAttribute('data-domain');
		expect(uiDomain).toMatch(/^(global|[0-9a-f-]{36})$/);
		await expect(container).toHaveAttribute('data-object', 'person.persons');
		await expect(container).toHaveAttribute('data-action', 'list');
		await expect(container).toHaveAttribute('data-debug-url', /\/core\/api\/authz\/debug/);
		await expect(container).toHaveAttribute('data-base-revision', /.+/);
		await expect(page.locator('[data-policy-inspector]')).toHaveCount(0);

		const apiResponse = await page.request.get('/person/persons', {
			headers: { Accept: 'application/json', 'X-Request-ID': 'e2e-person-req' },
		});
		expect(apiResponse.status()).toBe(403);
		const body = await apiResponse.json();
		expect(body.object).toBe('person.persons');
		expect(body.action).toBe('list');
		expect(body.domain).toBe(uiDomain);
		expect(body.request_id).toBe('e2e-person-req');
		expect(body.base_revision).toBeTruthy();
	});
});
