import { test, expect } from '@playwright/test';
import { login, logout } from '../../fixtures/auth';
import { resetTestDatabase, seedScenario } from '../../fixtures/test-data';

test.describe('employees CRUD operations', () => {
	test.beforeAll(async ({ request }) => {
		// Reset database and seed with comprehensive data for employee management
		await resetTestDatabase(request, { reseedMinimal: false });
		await seedScenario(request, 'comprehensive');
	});

	test.beforeEach(async ({ page }) => {
		await page.setViewportSize({ width: 1280, height: 720 });
	});

	test.afterEach(async ({ page }) => {
		await logout(page);
	});

	test('displays employees list page', async ({ page }) => {
		await login(page, 'test@gmail.com', 'TestPass123!');

		await page.goto('/hrm/employees');
		await expect(page).toHaveURL(/\/hrm\/employees$/);

		// Check page title and main elements
		await expect(page.locator('h1')).toContainText('Employees');
		await expect(page.locator('a[href="/hrm/employees/new"]')).toBeVisible();

		// Check search and filter form
		await expect(page.locator('form input[name="name"]')).toBeVisible();
		await expect(page.locator('form select[name="limit"]')).toBeVisible();
	});

	test('blocks employees list for user without HRM permissions', async ({ page }) => {
		await login(page, 'nohrm@example.com', 'TestPass123!');

		const response = await page.goto('/hrm/employees');
		if (response) {
			expect([401, 403]).toContain(response.status());
		}
		await expect(page.getByText('Permission required', { exact: false })).toBeVisible();
		const container = page.locator('[data-authz-container]');
		await expect(container).toBeVisible();
		await expect(container).toHaveAttribute('data-domain', 'hrm');
		await expect(container).toHaveAttribute('data-object', 'hrm.employees');
		await expect(container).toHaveAttribute('data-action', 'list');
		await expect(container).toHaveAttribute('data-request-url', '/core/api/authz/requests');
		await expect(container).toHaveAttribute('data-base-revision', /.+/);
		await expect(page.locator('[data-policy-inspector]')).toHaveCount(0);
		const applyButton = page.getByRole('button', { name: /Request access/i });
		if (await applyButton.count()) {
			await expect(applyButton).toBeVisible();
		} else {
			await expect(page.getByRole('link', { name: /Request access/i })).toBeVisible();
		}

		const apiResponse = await page.request.get('/hrm/employees', {
			headers: { Accept: 'application/json', 'X-Request-ID': 'e2e-hrm-req' },
		});
		expect(apiResponse.status()).toBe(403);
		const body = await apiResponse.json();
		expect(body.object).toBe('hrm.employees');
		expect(body.action).toBe('list');
		expect(body.domain).toBe('hrm');
		expect(body.request_id).toBe('e2e-hrm-req');
		expect(body.base_revision).toBeTruthy();
	});
});
