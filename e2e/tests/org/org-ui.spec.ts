import { test, expect } from '@playwright/test';
import { assertAuthenticated, login } from '../../fixtures/auth';
import { checkTestEndpointsHealth, resetTestDatabase, seedScenario } from '../../fixtures/test-data';

const ADMIN = {
	email: 'test@gmail.com',
	password: 'TestPass123!',
};

const READONLY = {
	email: 'org.readonly@example.com',
	password: 'TestPass123!',
};

async function ensureSeeded({ request }: { request: any }) {
	await checkTestEndpointsHealth(request);
	await resetTestDatabase(request);
	await seedScenario(request, 'org');
}

test.describe('Org UI (DEV-PLAN-035)', () => {
	test.beforeEach(async ({ request }) => {
		await ensureSeeded({ request });
	});

	test('管理员可创建节点并创建分配', async ({ page }) => {
		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		await page.goto('/org/nodes', { waitUntil: 'domcontentloaded' });
		await expect(page).toHaveURL(/\/org\/nodes/);

		await expect(page.evaluate(() => typeof (window as any).htmx !== 'undefined')).resolves.toBe(true);

		const newNodeBtn = page.locator('[data-testid="org-new-node"]');
		await expect(newNodeBtn).toBeVisible();
		const hxGet = await newNodeBtn.getAttribute('hx-get');
		expect(hxGet).toBeTruthy();

		const responsePromise = page.waitForResponse((resp) => resp.url().includes('/org/nodes/new'));
		await newNodeBtn.click();
		const resp = await responsePromise;
		expect(resp.status()).toBe(200);

		await expect(page.getByText('Create node', { exact: true })).toBeVisible();

		await page.locator('input[name="code"]').fill('ROOT');
		await page.locator('input[name="name"]').fill('Company');
		await page.getByRole('button', { name: 'Create' }).click();

		const tree = page.locator('#org-tree');
		await expect(tree.getByRole('button', { name: /Company/ })).toBeVisible();

		await page.locator('[data-testid="org-new-child"]').click();
		await expect(page.getByText('Create node', { exact: true })).toBeVisible();
		await page.locator('input[name="code"]').fill('HR');
		await page.locator('input[name="name"]').fill('HR Team');
		await page.getByRole('button', { name: 'Create' }).click();
		await expect(tree.getByRole('button', { name: /HR Team/ })).toBeVisible();

		await page.getByRole('link', { name: 'Assignments' }).click();
		await expect(page).toHaveURL(/\/org\/assignments/);

		await page.getByLabel('Pernr').fill('0001');
		await expect(page.locator('#org-pernr')).toHaveValue('0001');
		const orgNodeCombobox = page.locator('[data-testid="org-assignment-orgnode-combobox"]');
		await orgNodeCombobox.getByRole('textbox').fill('HR Team');
		const orgNodeSelect = orgNodeCombobox.locator('select[name="org_node_id"]');
		const firstSelectOption = orgNodeSelect.locator('option').first();
		await firstSelectOption.waitFor({ state: 'attached', timeout: 15_000 });
		const firstValue = await firstSelectOption.getAttribute('value');
		expect(firstValue).toBeTruthy();
		await orgNodeSelect.evaluate((el, value) => {
			const select = el as HTMLSelectElement;
			select.value = value as string;
			select.dispatchEvent(new Event('change', { bubbles: true }));
		}, firstValue);
		await expect(orgNodeSelect).toHaveValue(firstValue!);

		const createAssignmentResponse = page.waitForResponse((resp) => {
			return resp.request().method() === 'POST' && resp.url().includes('/org/assignments');
		});
		await page.getByRole('button', { name: 'Create' }).click();
		expect((await createAssignmentResponse).status()).toBe(200);
		await expect(page.locator('#org-assignments-timeline')).toContainText('0001');
	});

	test('无 Org 权限账号访问 /org/nodes 返回 Unauthorized', async ({ page }) => {
		await login(page, READONLY.email, READONLY.password);
		await assertAuthenticated(page);

		await page.goto('/org/nodes', { waitUntil: 'domcontentloaded' });
		const response = await page.request.get('/org/nodes', {
			headers: { Accept: 'application/json', 'X-Request-ID': 'e2e-org-nodes-deny' },
		});
		expect([401, 403]).toContain(response.status());

		await expect(page.getByRole('heading', { name: /Permission required/i, level: 2 })).toBeVisible();
		await expect(page.locator('section[data-authz-container]')).toBeVisible();
	});
});
