import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';
import path from 'path';
import { login, logout } from '../../fixtures/auth';
import { checkTestEndpointsHealth, resetTestDatabase, seedScenario } from '../../fixtures/test-data';

type TestDBConfig = {
	host?: string;
	port?: string;
	name?: string;
	user?: string;
};

type TestHealthResponse = {
	config?: {
		database?: TestDBConfig;
	};
};

function looksLikeUUID(value: string) {
	return /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i.test(
		value
	);
}

function seedOrg036ManufacturingDataset(dbConfig?: TestDBConfig) {
	const tenantID = '00000000-0000-0000-0000-000000000001';
	const projectRoot = path.resolve(__dirname, '../../..');
	const outputDir = process.env.ORG_DATA_MANIFEST_DIR || '/tmp/org-data-import';

	try {
		const dbName = dbConfig?.name || process.env.DB_NAME || 'iota_erp_e2e';
		const dbHost = dbConfig?.host || process.env.DB_HOST || 'localhost';
		const dbPort = dbConfig?.port || process.env.DB_PORT || '5432';
		const dbUser = dbConfig?.user || process.env.DB_USER || 'postgres';
		const dbPassword = process.env.DB_PASSWORD || 'postgres';

		execFileSync(
			'go',
			[
				'run',
				'./cmd/org-data',
				'import',
				'--tenant',
				tenantID,
				'--input',
				'docs/samples/org/036-manufacturing',
				'--output',
				outputDir,
				'--apply',
				'--skip-assignments',
			],
			{
				cwd: projectRoot,
				env: {
					...process.env,
					DB_NAME: dbName,
					DB_HOST: dbHost,
					DB_PORT: dbPort,
					DB_USER: dbUser,
					DB_PASSWORD: dbPassword,
				},
				stdio: 'pipe',
				encoding: 'utf8',
			}
		);
	} catch (err) {
		const error = err as any;
		const stdout = error?.stdout ? String(error.stdout) : '';
		const stderr = error?.stderr ? String(error.stderr) : '';
		throw new Error(
			`org-data import failed.\nstdout:\n${stdout}\nstderr:\n${stderr}`
		);
	}
}

	async function pickComboboxOption(
		containerSelector: string,
		page: any,
		keyword: string,
		optionText: RegExp
	) {
		const container = page.locator(containerSelector).first();
		const input = container.locator('input').first();
		await input.dispatchEvent('focus');
		await input.click();
		await input.fill(keyword);
		await input.dispatchEvent('change');

		const option = container.locator('.combobox-option', { hasText: optionText }).first();
		await option.waitFor({ state: 'visible', timeout: 15_000 });
		await option.click();
		await page.waitForTimeout(250);
	}

	test.describe('person detail page assignment UX', () => {
		test.beforeAll(async ({ request }) => {
			await resetTestDatabase(request, { reseedMinimal: false });
			await seedScenario(request, 'comprehensive');

			const health = (await checkTestEndpointsHealth(request)) as TestHealthResponse;
			seedOrg036ManufacturingDataset(health.config?.database);
		});

	test.beforeEach(async ({ page }) => {
		await page.setViewportSize({ width: 1440, height: 900 });
	});

	test.afterEach(async ({ page }) => {
		await logout(page);
	});

	test('can update 061001 assignment with department-scoped position selection', async ({
		page,
	}) => {
		await login(page, 'test@gmail.com', 'TestPass123!');

		// Create person 061001 first (e2e dataset does not seed persons).
		await page.goto('/person/persons/new');
		await page.locator('input[name="Pernr"]').fill('061001');
		await page.locator('input[name="DisplayName"]').fill('Ava Reed');
		await Promise.all([
			page.waitForURL(/\/person\/persons\/[0-9a-f-]+/i, { timeout: 15_000 }),
			page.locator('form button[type="submit"]').click(),
		]);

		// Creation should guide the user to fill assignment.
		await expect(page).toHaveURL(/step=assignment/);

		const assignmentsTimeline = page.locator('#org-assignments-timeline');
		await assignmentsTimeline.waitFor({ state: 'visible', timeout: 15_000 });

		const summary = page.locator('#person-current-assignment');
		await expect(summary).toBeVisible();
		await expect(summary).not.toContainText(/Loading|加载中/i, { timeout: 15_000 });

		const assignmentForm = page.locator('#org-assignment-form');
		await expect(assignmentForm.locator('form')).toBeVisible({ timeout: 15_000 });

			const positionInput = page
				.locator('[data-testid="org-assignment-position-combobox"] input')
				.first();
			await expect(positionInput).toBeDisabled();

		// Select department: 房地产 / Real Estate
			await pickComboboxOption(
				'[data-testid="org-assignment-orgnode-combobox"]',
				page,
				'房',
				/房地产|Real Estate/i
			);
			await expect(page.locator('#org-assignment-form select[name="org_node_id"]')).not.toHaveValue(
				''
			);
			await expect(positionInput).toBeEnabled();

			// position should be empty before selection
			await expect(page.locator('#org-assignment-form select[name="position_id"]')).toHaveValue(
				''
		);

		// pick a department-scoped position (should belong to 房地产)
		await pickComboboxOption(
			'[data-testid="org-assignment-position-combobox"]',
			page,
			'总经理',
			/房地产.*总经理|POS-RE-CEO|CEO/i
		);

		await expect(page.locator('#org-assignment-form select[name="position_id"]')).not.toHaveValue(
			''
		);

		await assignmentForm.locator('form button[type="submit"]').click();

		// timeline should show the new department label
		await expect(assignmentsTimeline).toContainText(/房地产|Real Estate/i, { timeout: 15_000 });
		await expect(assignmentsTimeline.locator('table')).toBeVisible({ timeout: 15_000 });

		// org/position columns should not display raw UUIDs
		const labelCells = page.locator(
			'#org-assignments-timeline tbody tr td:nth-child(3), #org-assignments-timeline tbody tr td:nth-child(4)'
		);
		const cellCount = await labelCells.count();
		expect(cellCount).toBeGreaterThan(0);
		for (let i = 0; i < cellCount; i++) {
			const text = (await labelCells.nth(i).textContent()) || '';
			expect(looksLikeUUID(text)).toBeFalsy();
		}

		await expect(summary).toContainText(/房地产|Real Estate/i);
	});

	test('new person page hints that department/position is set after creation', async ({
		page,
	}) => {
		await login(page, 'test@gmail.com', 'TestPass123!');
		await page.goto('/person/persons/new');
		await expect(page.locator('form')).toBeVisible();
		await expect(page.locator('form')).toContainText(/department|position|部门|职位/i);
	});
});
