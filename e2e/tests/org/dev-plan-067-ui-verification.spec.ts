import { test, expect, type Page } from '@playwright/test';
import * as path from 'path';
import * as fs from 'fs';
import { assertAuthenticated, login, logout } from '../../fixtures/auth';
import { checkTestEndpointsHealth, resetTestDatabase, seedScenario } from '../../fixtures/test-data';

const ADMIN = {
	email: 'test@gmail.com',
	password: 'TestPass123!',
};

const NO_ORG_ACCESS = {
	email: 'org.readonly@example.com',
	password: 'TestPass123!',
};

async function ensureSeeded({ request }: { request: any }) {
	await checkTestEndpointsHealth(request);
	await resetTestDatabase(request);
	await seedScenario(request, 'org');
}

async function ensureJobCatalogPath(args: { page: Page }) {
	const effectiveDate = '2025-01-01';
	const groupCode = 'FG-001';
	const familyCode = 'F-001';
	const profileCode = 'JP-001';
	const levelCode = 'L-001';

	const createGroup = await args.page.request.post('/org/api/job-catalog/family-groups', {
		data: {
			code: groupCode,
			name: 'E2E Job family group',
			is_active: true,
			effective_date: effectiveDate,
		},
		failOnStatusCode: false,
	});
	expect(createGroup.status()).toBe(201);
	const group = await createGroup.json();

	const createFamily = await args.page.request.post('/org/api/job-catalog/families', {
		data: {
			job_family_group_id: group.id,
			code: familyCode,
			name: 'E2E Job family',
			is_active: true,
			effective_date: effectiveDate,
		},
		failOnStatusCode: false,
	});
	expect(createFamily.status()).toBe(201);
	const family = await createFamily.json();

	const createProfile = await args.page.request.post('/org/api/job-profiles', {
		data: {
			code: profileCode,
			name: 'E2E Job profile',
			job_families: [
				{
					job_family_id: family.id,
					is_primary: true,
				},
			],
			is_active: true,
			effective_date: effectiveDate,
		},
		failOnStatusCode: false,
	});
	expect(createProfile.status()).toBe(201);
	const profile = await createProfile.json();

	const createLevel = await args.page.request.post('/org/api/job-catalog/levels', {
		data: {
			code: levelCode,
			name: 'E2E Job level',
			display_order: 1,
			is_active: true,
			effective_date: effectiveDate,
		},
		failOnStatusCode: false,
	});
	expect(createLevel.status()).toBe(201);

	return { groupCode, familyCode, profileCode, profileID: profile.id, levelCode };
}

async function setComboboxValue(args: { combobox: ReturnType<Page['locator']>; query: string; value: string }) {
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

function screenshotDir() {
	const repoRoot = path.resolve(process.cwd(), '..');
	return path.join(repoRoot, 'tmp', 'ui-verification', 'dev-plan-067', 'latest');
}

async function saveScreenshot(args: { page: Page; name: string; fullPage?: boolean }) {
	const outDir = screenshotDir();
	fs.mkdirSync(outDir, { recursive: true });
	const filePath = path.join(outDir, `${args.name}.png`);
	await args.page.screenshot({ path: filePath, fullPage: args.fullPage ?? true });
	return filePath;
}

async function createJobCatalogRowsViaUI(args: { page: Page; viewportName: string }) {
	const groupCode = 'FG-UI-001';
	const familyCode = 'F-UI-001';
	const profileCode = 'JP-UI-001';
	const levelCode = 'L-UI-001';

	const effectiveDate = await args.page.locator('#effective-date').inputValue();

	const groupForm = args.page
		.locator('form')
		.filter({ has: args.page.locator('input[name="tab"][value="family-groups"]') })
		.filter({ has: args.page.locator('input[name="code"]') })
		.first();
	await groupForm.locator('input[name="code"]').fill(groupCode);
	await groupForm.locator('input[name="name"]').fill('UI Job family group');
	const createGroupResp = args.page.waitForResponse((resp) => {
		return resp.request().method() === 'POST' && resp.url().includes('/org/job-catalog/family-groups');
	});
	await groupForm.getByRole('button', { name: 'Save', exact: true }).click();
	expect((await createGroupResp).status()).toBe(200);
	await args.page.waitForLoadState('domcontentloaded');

	const groupRow = args.page.locator('table tbody tr', { hasText: groupCode }).first();
	await expect(groupRow).toBeVisible();
	await saveScreenshot({ page: args.page, name: `org-job-catalog-family-groups-created--${args.viewportName}` });

	await groupRow.getByRole('link', { name: 'Edit', exact: true }).click();
	await args.page.waitForLoadState('domcontentloaded');

	const editGroupForm = args.page
		.locator('form')
		.filter({ has: args.page.locator('input[name="tab"][value="family-groups"]') })
		.filter({ has: args.page.locator('input[name="edit_id"]') })
		.first();
	await expect(editGroupForm.locator('input[name="code"]')).toHaveValue(groupCode);

	const writeModeSelect = args.page.locator('#org-job-catalog-write-mode');
	await expect(writeModeSelect).toBeVisible();
	await writeModeSelect.selectOption('correct');

	await editGroupForm.locator('input[name="name"]').fill('UI Job family group (updated)');
	const updateGroupResp = args.page.waitForResponse((resp) => {
		return resp.request().method() === 'PATCH' && resp.url().includes('/org/job-catalog/family-groups/');
	});
	await editGroupForm.getByRole('button', { name: 'Save', exact: true }).click();
	expect((await updateGroupResp).status()).toBe(200);
	await args.page.waitForLoadState('domcontentloaded');

	const updatedGroupRow = args.page.locator('table tbody tr', { hasText: groupCode }).first();
	await expect(updatedGroupRow).toContainText('UI Job family group (updated)');
	await saveScreenshot({ page: args.page, name: `org-job-catalog-family-groups-updated--${args.viewportName}` });

	await args.page.goto(
		`/org/job-catalog?tab=families&effective_date=${encodeURIComponent(effectiveDate)}&job_family_group_code=${encodeURIComponent(groupCode)}`,
		{ waitUntil: 'domcontentloaded' }
	);
	await expect(args.page.locator('#org-job-catalog-page')).toBeVisible();
	const familyForm = args.page
		.locator('form')
		.filter({ has: args.page.locator('input[name="tab"][value="families"]') })
		.filter({ has: args.page.locator('input[name="code"]') })
		.first();
	await familyForm.locator('input[name="code"]').fill(familyCode);
	await familyForm.locator('input[name="name"]').fill('UI Job family');
	const createFamilyResp = args.page.waitForResponse((resp) => {
		return resp.request().method() === 'POST' && resp.url().includes('/org/job-catalog/families');
	});
	await familyForm.getByRole('button', { name: 'Save', exact: true }).click();
	expect((await createFamilyResp).status()).toBe(200);
	await args.page.waitForLoadState('domcontentloaded');
	await expect(args.page.locator('table tbody tr', { hasText: familyCode }).first()).toBeVisible();
	await saveScreenshot({ page: args.page, name: `org-job-catalog-families-created--${args.viewportName}` });

	const groupListResp = await args.page.request.get('/org/api/job-catalog/family-groups', { failOnStatusCode: false });
	expect(groupListResp.status()).toBe(200);
	const groupList = await groupListResp.json();
	const group = (groupList.items ?? []).find((it: any) => it.code === groupCode);
	expect(group).toBeTruthy();

	const familyListResp = await args.page.request.get(
		`/org/api/job-catalog/families?job_family_group_id=${encodeURIComponent(group.id)}`,
		{ failOnStatusCode: false }
	);
	expect(familyListResp.status()).toBe(200);
	const familyList = await familyListResp.json();
	const family = (familyList.items ?? []).find((it: any) => it.code === familyCode);
	expect(family).toBeTruthy();

	await args.page.goto(`/org/job-catalog?tab=profiles&effective_date=${encodeURIComponent(effectiveDate)}`, {
		waitUntil: 'domcontentloaded',
	});
	await expect(args.page.locator('#org-job-catalog-page')).toBeVisible();
	const profileForm = args.page
		.locator('form')
		.filter({ has: args.page.locator('input[name="tab"][value="profiles"]') })
		.filter({ has: args.page.locator('input[name="code"]') })
		.first();
		await profileForm.locator('input[name="code"]').fill(profileCode);
		await profileForm.locator('input[name="name"]').fill('UI Job profile');

		const family0Combobox = profileForm.locator('div[x-data^="combobox("]').first();
		await setComboboxValue({ combobox: family0Combobox, query: familyCode, value: family.id });
		await profileForm.locator('input[type="radio"][name="primary_index"][value="0"]').check();

	const createProfileResp = args.page.waitForResponse((resp) => {
		return resp.request().method() === 'POST' && resp.url().includes('/org/job-catalog/profiles');
	});
	await profileForm.getByRole('button', { name: 'Save', exact: true }).click();
	expect((await createProfileResp).status()).toBe(200);
	await args.page.waitForLoadState('domcontentloaded');
	await expect(args.page.locator('table tbody tr', { hasText: profileCode }).first()).toBeVisible();
	await saveScreenshot({ page: args.page, name: `org-job-catalog-profiles-created--${args.viewportName}` });

	await args.page.goto(
		`/org/job-catalog?tab=levels&effective_date=${encodeURIComponent(effectiveDate)}&job_family_group_code=${encodeURIComponent(groupCode)}`,
		{ waitUntil: 'domcontentloaded' }
	);
	await expect(args.page.locator('#org-job-catalog-page')).toBeVisible();
	const levelForm = args.page
		.locator('form')
		.filter({ has: args.page.locator('input[name="tab"][value="levels"]') })
		.filter({ has: args.page.locator('input[name="code"]') })
		.first();
	await levelForm.locator('input[name="code"]').fill(levelCode);
	await levelForm.locator('input[name="name"]').fill('UI Job level');
	await levelForm.locator('input[name="display_order"]').fill('9');
	const createLevelResp = args.page.waitForResponse((resp) => {
		return resp.request().method() === 'POST' && resp.url().includes('/org/job-catalog/levels');
	});
	await levelForm.getByRole('button', { name: 'Save', exact: true }).click();
	expect((await createLevelResp).status()).toBe(200);
	await args.page.waitForLoadState('domcontentloaded');
	await expect(args.page.locator('table tbody tr', { hasText: levelCode }).first()).toBeVisible();
	await saveScreenshot({ page: args.page, name: `org-job-catalog-levels-created--${args.viewportName}` });
}

const VIEWPORTS = [
	{ name: 'mobile', width: 390, height: 844 },
	{ name: 'laptop', width: 1366, height: 768 },
	{ name: 'desktop', width: 1920, height: 1080 },
];

for (const viewport of VIEWPORTS) {
	test.describe(`DEV-PLAN-067 UI verification (${viewport.name})`, () => {
		test.use({ viewport: { width: viewport.width, height: viewport.height } });

		test.beforeEach(async ({ request }) => {
			await ensureSeeded({ request });
		});

		test('pages + evidence (DEV-PLAN-044)', async ({ page }) => {
			await login(page, ADMIN.email, ADMIN.password);
			await assertAuthenticated(page);

			const jobCatalog = await ensureJobCatalogPath({ page });

			// /org/nodes (Org structure)
			await page.goto('/org/nodes', { waitUntil: 'domcontentloaded' });
			await expect(page.locator('#org-tree')).toBeVisible();
			await saveScreenshot({ page, name: `org-nodes--${viewport.name}` });

			// Create a node (for positions screenshots)
			await page.locator('[data-testid="org-new-node"]').click();
			await expect(page.getByText('Create node', { exact: true })).toBeVisible();
			await page.locator('input[name="code"]').fill('ROOT');
			await page.locator('input[name="name"]').fill('Company');
			await page.getByRole('button', { name: 'Create' }).click();
			const tree = page.locator('#org-tree');
			await expect(tree.getByRole('button', { name: /Company/ })).toBeVisible();

			// /org/positions (Position management)
			await page.goto('/org/positions', { waitUntil: 'domcontentloaded' });
			await expect(page.locator('#org-tree')).toBeVisible();
			await saveScreenshot({ page, name: `org-positions--${viewport.name}` });

			// Open Create position form
			await tree.getByRole('button', { name: /Company/ }).click();
			await page.getByRole('button', { name: 'Create position', exact: true }).click();
			await expect(page.locator('#org-position-details').getByText('Create position', { exact: true })).toBeVisible();

			// Org node combobox should not duplicate selected chips after HTMX swaps.
			const positionForm = page.locator('#org-position-form');
			const positionNodeID = await positionForm.locator('input[name="node_id"]').inputValue();
			await setComboboxValue({
				combobox: positionForm.locator('[data-testid="org-position-orgnode-combobox"]'),
				query: 'Company',
				value: positionNodeID,
			});
			await expect(
				positionForm.locator('[data-testid="org-position-orgnode-combobox"]').getByRole('button', { name: 'Remove' })
			).toHaveCount(1);

			await positionForm.locator('input[name="code"]').fill('POS-001');
				await positionForm.locator('input[name="title"]').fill('HR Specialist');
				await positionForm.locator('input[name="position_type"]').fill('regular');
				await positionForm.locator('input[name="employment_type"]').fill('full_time');

				await setComboboxValue({
					combobox: positionForm.locator('[data-testid="org-position-job-profile-id-combobox"]'),
					query: jobCatalog.profileCode,
					value: jobCatalog.profileID,
				});
				await setComboboxValue({
					combobox: positionForm.locator('[data-testid="org-position-job-level-code-combobox"]'),
					query: jobCatalog.levelCode,
				value: jobCatalog.levelCode,
			});

			await saveScreenshot({ page, name: `org-positions-create--${viewport.name}` });

			// /org/job-catalog (Job Catalog)
			await page.goto('/org/job-catalog?tab=family-groups', { waitUntil: 'domcontentloaded' });
			await expect(page.locator('#org-job-catalog-page')).toBeVisible();
			await saveScreenshot({ page, name: `org-job-catalog-family-groups--${viewport.name}` });

			if (viewport.name === 'laptop') {
				await createJobCatalogRowsViaUI({ page, viewportName: viewport.name });
			}

			await page.goto(
				`/org/job-catalog?tab=families&job_family_group_code=${encodeURIComponent(jobCatalog.groupCode)}`,
				{ waitUntil: 'domcontentloaded' }
			);
			await expect(page.locator('#org-job-catalog-page')).toBeVisible();
			await saveScreenshot({ page, name: `org-job-catalog-families--${viewport.name}` });

				await page.goto(
					`/org/job-catalog?tab=profiles&job_family_group_code=${encodeURIComponent(jobCatalog.groupCode)}`,
					{ waitUntil: 'domcontentloaded' }
				);
				await expect(page.locator('#org-job-catalog-page')).toBeVisible();
				await saveScreenshot({ page, name: `org-job-catalog-profiles--${viewport.name}` });

				await page.goto(
					`/org/job-catalog?tab=levels&job_family_group_code=${encodeURIComponent(jobCatalog.groupCode)}`,
					{ waitUntil: 'domcontentloaded' }
				);
				await expect(page.locator('#org-job-catalog-page')).toBeVisible();
				await saveScreenshot({ page, name: `org-job-catalog-levels--${viewport.name}` });

			// Authz gate evidence (no org access)
			await logout(page);
			await login(page, NO_ORG_ACCESS.email, NO_ORG_ACCESS.password);
			await assertAuthenticated(page);

			await page.goto('/org/job-catalog', { waitUntil: 'domcontentloaded' });
			await expect(page.getByRole('heading', { name: /Permission required/i, level: 2 })).toBeVisible();
			await saveScreenshot({ page, name: `org-job-catalog-unauthorized--${viewport.name}` });
		});
	});
}
