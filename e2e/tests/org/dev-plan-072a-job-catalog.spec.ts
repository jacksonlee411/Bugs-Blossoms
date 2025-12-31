import { test, expect, type Page } from '@playwright/test';
import { assertAuthenticated, login } from '../../fixtures/auth';
import { checkTestEndpointsHealth, resetTestDatabase, seedScenario } from '../../fixtures/test-data';

const ADMIN = {
	email: 'test@gmail.com',
	password: 'TestPass123!',
};

async function ensureSeeded({ request }: { request: any }) {
	await checkTestEndpointsHealth(request);
	await resetTestDatabase(request);
	await seedScenario(request, 'org');
}

async function createJobFamilyGroup(args: { page: Page; code: string }) {
	const resp = await args.page.request.post('/org/api/job-catalog/family-groups', {
		data: {
			code: args.code,
			name: 'E2E Job family group',
			is_active: true,
		},
		failOnStatusCode: false,
	});
	expect(resp.status()).toBe(201);
}

test.describe('Org Job Catalog (DEV-PLAN-072A)', () => {
	test.beforeEach(async ({ request }) => {
		await ensureSeeded({ request });
	});

	test('families tab 未指定 group_code 时自动选中 active 职类（072A）', async ({ page }) => {
		const groupCode = 'FG-001';

		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		await createJobFamilyGroup({ page, code: groupCode });

		await page.goto('/org/job-catalog?tab=families', { waitUntil: 'domcontentloaded' });
		await expect(page.locator('#org-job-catalog-page')).toBeVisible();

		await expect(page.locator(`input[name="job_family_group_code"][value="${groupCode}"]`)).toHaveCount(1);
		await expect(
			page.locator(`form#org-job-catalog-filters select option[value="${groupCode}"][selected]`),
		).toHaveCount(1);
	});
});
