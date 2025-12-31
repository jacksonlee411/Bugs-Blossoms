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

async function setComboboxValue(args: {
	combobox: ReturnType<Page['locator']>;
	query: string;
	value: string;
}) {
	const textbox = args.combobox.getByRole('textbox');
	await expect(textbox).toBeEnabled();
	await textbox.click();
	await textbox.fill(args.query);
	const select = args.combobox.locator('select');
	const option = select.locator(`option[value="${args.value}"]`).first();
	await option.waitFor({ state: 'attached', timeout: 15_000 });
	await select.evaluate((el, value) => {
		const select = el as HTMLSelectElement;
		select.value = value as string;
		for (const opt of Array.from(select.options)) {
			opt.selected = opt.value === (value as string);
		}
		select.dispatchEvent(new Event('change', { bubbles: true }));
	}, args.value);
	await textbox.press('Escape');
}

test.describe('Org Job Catalog profiles (HTMX UX)', () => {
	test.beforeEach(async ({ request }) => {
		await ensureSeeded({ request });
	});

	test('保存错误时不应把整页框架插入到局部容器（profiles tab）', async ({ page }) => {
		const effectiveDate = '2025-12-31';
		const profileCode = 'JP-HTMX-ERR-001';

		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		const createGroup = await page.request.post('/org/api/job-catalog/family-groups', {
			data: { code: 'G1', name: 'Group 1', is_active: true },
			failOnStatusCode: false,
		});
		expect(createGroup.status()).toBe(201);
		const group = await createGroup.json();

		const createFamily = await page.request.post('/org/api/job-catalog/families', {
			data: { job_family_group_id: group.id, code: 'F1', name: 'Family 1', is_active: true },
			failOnStatusCode: false,
		});
		expect(createFamily.status()).toBe(201);
		const family = await createFamily.json();

		const createFamily2 = await page.request.post('/org/api/job-catalog/families', {
			data: { job_family_group_id: group.id, code: 'F2', name: 'Family 2', is_active: true },
			failOnStatusCode: false,
		});
		expect(createFamily2.status()).toBe(201);
		const family2 = await createFamily2.json();

		await page.goto(
			`/org/job-catalog?tab=profiles&effective_date=${encodeURIComponent(effectiveDate)}&job_family_group_code=G1`,
			{ waitUntil: 'domcontentloaded' },
		);
		await expect(page.locator('#org-job-catalog-page')).toBeVisible();

		const profileForm = page
			.locator('form')
			.filter({ has: page.locator('input[name="tab"][value="profiles"]') })
			.filter({ has: page.locator('input[name="code"]') })
			.first();

		await profileForm.locator('input[name="code"]').fill(profileCode);
		await profileForm.locator('input[name="name"]').fill('HTMX Error Profile');

		const family0Combobox = profileForm.locator('div[x-data^="combobox("]').first();
		await setComboboxValue({ combobox: family0Combobox, query: 'F1', value: family.id });

		const family1Combobox = profileForm.locator('div[x-data^="combobox("]').nth(1);
		await setComboboxValue({ combobox: family1Combobox, query: 'F2', value: family2.id });

		const saveResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'POST' && resp.url().includes('/org/job-catalog/profiles');
		});
		await profileForm.getByRole('button', { name: 'Save', exact: true }).click();
		expect((await saveResp).status()).toBe(422);

		await expect(page.locator('#org-job-catalog-page')).toContainText('primary_index is required');
		await expect(page.locator('#org-job-catalog-page [aria-label="Sidebar navigation"]')).toHaveCount(0);
	});

	test('仅填写一个职种且占比 100 时，未选择 primary 也应可保存（profiles tab）', async ({ page }) => {
		const effectiveDate = '2025-12-31';
		const profileCode = 'JP-HTMX-OK-001';

		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		const createGroup = await page.request.post('/org/api/job-catalog/family-groups', {
			data: { code: 'G1', name: 'Group 1', is_active: true },
			failOnStatusCode: false,
		});
		expect(createGroup.status()).toBe(201);
		const group = await createGroup.json();

		const createFamily = await page.request.post('/org/api/job-catalog/families', {
			data: { job_family_group_id: group.id, code: 'F1', name: 'Family 1', is_active: true },
			failOnStatusCode: false,
		});
		expect(createFamily.status()).toBe(201);
		const family = await createFamily.json();

		await page.goto(
			`/org/job-catalog?tab=profiles&effective_date=${encodeURIComponent(effectiveDate)}&job_family_group_code=G1`,
			{ waitUntil: 'domcontentloaded' },
		);
		await expect(page.locator('#org-job-catalog-page')).toBeVisible();

		const profileForm = page
			.locator('form')
			.filter({ has: page.locator('input[name="tab"][value="profiles"]') })
			.filter({ has: page.locator('input[name="code"]') })
			.first();

		await profileForm.locator('input[name="code"]').fill(profileCode);
		await profileForm.locator('input[name="name"]').fill('HTMX OK Profile');

		const family0Combobox = profileForm.locator('div[x-data^="combobox("]').first();
		await setComboboxValue({ combobox: family0Combobox, query: 'F1', value: family.id });

		const saveResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'POST' && resp.url().includes('/org/job-catalog/profiles');
		});
		await profileForm.getByRole('button', { name: 'Save', exact: true }).click();
		expect((await saveResp).status()).toBe(200);

		await page.waitForLoadState('domcontentloaded');
		await expect(page.locator('table tbody tr', { hasText: profileCode }).first()).toBeVisible();
	});
});
