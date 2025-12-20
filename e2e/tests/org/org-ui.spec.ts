import { test, expect, type Locator } from '@playwright/test';
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

function formatUTCDate(date: Date) {
	const yyyy = date.getUTCFullYear();
	const mm = String(date.getUTCMonth() + 1).padStart(2, '0');
	const dd = String(date.getUTCDate()).padStart(2, '0');
	return `${yyyy}-${mm}-${dd}`;
}

function parseUTCDateString(dateStr: string) {
	return new Date(`${dateStr}T00:00:00Z`);
}

function addUTCDays(date: Date, days: number) {
	const out = new Date(date);
	out.setUTCDate(out.getUTCDate() + days);
	return out;
}

async function setComboboxValue(args: {
	combobox: Locator;
	query: string;
	value: string;
}) {
	const textbox = args.combobox.getByRole('textbox');
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

test.describe('Org UI (DEV-PLAN-035)', () => {
	test.beforeEach(async ({ request }) => {
		await ensureSeeded({ request });
	});

	test('管理员可创建/编辑/移动节点，并创建/编辑分配', async ({ page }) => {
		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		await page.goto('/org/nodes', { waitUntil: 'domcontentloaded' });
		await expect(page).toHaveURL(/\/org\/nodes/);

		await page.waitForFunction(() => typeof (window as any).htmx !== 'undefined', { timeout: 10_000 });

		const nodesBaseEffectiveDateStr = await page.locator('#effective-date').inputValue();
		expect(nodesBaseEffectiveDateStr).toMatch(/^\d{4}-\d{2}-\d{2}$/);
		const nodesBaseEffectiveDate = parseUTCDateString(nodesBaseEffectiveDateStr);

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

		const newChildBtnFromRoot = page.locator('[data-testid="org-new-child"]');
		await expect(newChildBtnFromRoot).toBeVisible();
		const newChildHxGet = await newChildBtnFromRoot.getAttribute('hx-get');
		expect(newChildHxGet).toMatch(/parent_id=/);
		const companyID = new URL(`http://local${newChildHxGet}`).searchParams.get('parent_id');
		expect(companyID).toBeTruthy();
		const companyIDValue = companyID!;

		await page.locator('[data-testid="org-new-child"]').click();
		await expect(page.getByText('Create node', { exact: true })).toBeVisible();
		await page.locator('input[name="code"]').fill('HR');
		await page.locator('input[name="name"]').fill('HR Team');
		const createHRResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'POST' && resp.url().includes('/org/nodes');
		});
		await page.getByRole('button', { name: 'Create' }).click();
		expect((await createHRResp).status()).toBe(200);
		await expect(tree.getByRole('button', { name: /HR Team/ })).toBeVisible();

		await tree.getByRole('button', { name: /Company/ }).click();
		await page.locator('[data-testid="org-new-child"]').click();
		await expect(page.getByText('Create node', { exact: true })).toBeVisible();
		await page.locator('input[name="code"]').fill('IT');
		await page.locator('input[name="name"]').fill('IT Team');
		const createITResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'POST' && resp.url().includes('/org/nodes');
		});
		await page.getByRole('button', { name: 'Create' }).click();
		expect((await createITResp).status()).toBe(200);
		await expect(tree.getByRole('button', { name: /IT Team/ })).toBeVisible();

		const nodesEditDate = addUTCDays(nodesBaseEffectiveDate, 1);
		const nodesEditDateStr = formatUTCDate(nodesEditDate);
		const nodesEffectiveDateChangeResp = page.waitForResponse((resp) => {
			return (
				resp.request().method() === 'GET' &&
				resp.url().includes('/org/nodes') &&
				resp.url().includes(`effective_date=${nodesEditDateStr}`)
			);
		});
		const nodesEffectiveDateInput = page.locator('#effective-date');
		await nodesEffectiveDateInput.fill(nodesEditDateStr);
		await nodesEffectiveDateInput.dispatchEvent('change');
		expect((await nodesEffectiveDateChangeResp).status()).toBe(200);

		const itDetailsResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'GET' && resp.url().includes('/org/nodes/');
		});
		await tree.getByRole('button', { name: /IT Team/ }).click();
		expect((await itDetailsResp).status()).toBe(200);
		await expect(page.locator('#org-node-panel')).toContainText('IT Team');
		await page.waitForURL(/node_id=/);
		const itID = new URL(page.url()).searchParams.get('node_id');
		expect(itID).toBeTruthy();
		const itIDValue = itID!;

		const hrDetailsResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'GET' && resp.url().includes('/org/nodes/');
		});
		await tree.getByRole('button', { name: /HR Team/ }).click();
		expect((await hrDetailsResp).status()).toBe(200);
		await expect(page.locator('#org-node-panel')).toContainText('HR Team');
		await page.waitForURL(/node_id=/);
		const hrID = new URL(page.url()).searchParams.get('node_id');
		expect(hrID).toBeTruthy();
		const hrIDValue = hrID!;
		await page.getByRole('button', { name: 'Edit' }).click();
		await expect(page.getByText('Edit node', { exact: true })).toBeVisible();
		await page.locator('input[name="name"]').fill('HR Team Updated');
		const updateNodeResp = page.waitForResponse((resp) => {
			return (
				resp.request().method() === 'PATCH' &&
				resp.url().includes(`/org/nodes/${hrIDValue}`) &&
				resp.url().includes(`effective_date=${nodesEditDateStr}`)
			);
		});
		await page.getByRole('button', { name: 'Save' }).click();
		expect((await updateNodeResp).status()).toBe(200);
		await expect(page.locator('#org-node-panel')).toContainText('HR Team Updated');
		await expect(tree.getByRole('button', { name: /HR Team Updated/ })).toBeVisible();

		const nodesMoveDate = addUTCDays(nodesBaseEffectiveDate, 2);
		const nodesMoveDateStr = formatUTCDate(nodesMoveDate);
		const nodesMoveEffectiveDateChangeResp = page.waitForResponse((resp) => {
			return (
				resp.request().method() === 'GET' &&
				resp.url().includes('/org/nodes') &&
				resp.url().includes(`effective_date=${nodesMoveDateStr}`)
			);
		});
		await nodesEffectiveDateInput.fill(nodesMoveDateStr);
		await nodesEffectiveDateInput.dispatchEvent('change');
		expect((await nodesMoveEffectiveDateChangeResp).status()).toBe(200);

		const hrDetailsAfterMoveDateResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'GET' && resp.url().includes(`/org/nodes/${hrIDValue}`) && resp.url().includes(`effective_date=${nodesMoveDateStr}`);
		});
		await tree.getByRole('button', { name: /HR Team Updated/ }).click();
		expect((await hrDetailsAfterMoveDateResp).status()).toBe(200);
		await expect(page.locator('#org-node-panel')).toContainText('HR Team Updated');

		await page.getByRole('button', { name: 'Change parent' }).click();
		await expect(page.getByText('Move node', { exact: true })).toBeVisible();
		const moveParentCombobox = page.locator('[data-testid="org-node-new-parent-combobox"]');
		const moveParentSelect = moveParentCombobox.locator('select[name="new_parent_id"]');
		await setComboboxValue({
			combobox: moveParentCombobox,
			query: 'IT Team',
			value: itIDValue,
		});
		await expect(moveParentSelect).toHaveValue(itIDValue);

		const moveNodeResp = page.waitForResponse(
			(resp) =>
				resp.request().method() === 'POST' &&
				resp.url().includes(`/org/nodes/${hrIDValue}:move`) &&
				resp.url().includes(`effective_date=${nodesMoveDateStr}`),
			{ timeout: 30_000 }
		);
		await page.getByRole('button', { name: 'Move', exact: true }).click();
		expect((await moveNodeResp).status()).toBe(200);
		await expect(page.locator('#org-node-panel')).toContainText(itIDValue, { timeout: 15_000 });

		await page.getByRole('link', { name: 'Assignments' }).click();
		await expect(page).toHaveURL(/\/org\/assignments/);

		const assignmentsBaseEffectiveDateStr = await page.locator('#effective-date').inputValue();
		expect(assignmentsBaseEffectiveDateStr).toMatch(/^\d{4}-\d{2}-\d{2}$/);
		const assignmentsBaseEffectiveDate = parseUTCDateString(assignmentsBaseEffectiveDateStr);

		await page.getByLabel('Pernr').fill('0001');
		await expect(page.locator('#org-pernr')).toHaveValue('0001');
		const orgNodeCombobox = page.locator('[data-testid="org-assignment-orgnode-combobox"]');
		const orgNodeSelect = orgNodeCombobox.locator('select[name="org_node_id"]');
		await setComboboxValue({
			combobox: orgNodeCombobox,
			query: 'HR Team Updated',
			value: hrIDValue,
		});
		await expect(orgNodeSelect).toHaveValue(hrIDValue);

		const createAssignmentResponse = page.waitForResponse((resp) => {
			return resp.request().method() === 'POST' && resp.url().includes('/org/assignments');
		});
		await page.getByRole('button', { name: 'Create' }).click();
		expect((await createAssignmentResponse).status()).toBe(200);
		await expect(page.locator('#org-assignments-timeline')).toContainText('0001');

		const assignmentsFuture = addUTCDays(assignmentsBaseEffectiveDate, 7);
		const assignmentsFutureStr = formatUTCDate(assignmentsFuture);
		const effectiveDateChangeResp = page.waitForResponse((resp) => {
			return (
				resp.request().method() === 'GET' &&
				resp.url().includes('/org/assignments') &&
				resp.url().includes(`effective_date=${assignmentsFutureStr}`)
			);
		});
		const effectiveDateInput = page.locator('#effective-date');
		await effectiveDateInput.fill(assignmentsFutureStr);
		await effectiveDateInput.dispatchEvent('change');
		expect((await effectiveDateChangeResp).status()).toBe(200);

		const editButton = page.locator('[data-testid^="org-assignment-edit-"]').first();
		await expect(editButton).toBeVisible();
		const editResp = page.waitForResponse((resp) => {
			return (
				resp.request().method() === 'GET' &&
				resp.url().includes('/org/assignments/') &&
				resp.url().includes('/edit')
			);
		});
		await editButton.click();
		expect((await editResp).status()).toBe(200);
		await expect(page.locator('[data-testid="org-assignment-cancel-edit"]')).toBeVisible();

		const editOrgNodeCombobox = page.locator('[data-testid="org-assignment-orgnode-combobox"]');
		const editOrgNodeSelect = editOrgNodeCombobox.locator('select[name="org_node_id"]');
		await setComboboxValue({
			combobox: editOrgNodeCombobox,
			query: 'Company',
			value: companyIDValue,
		});
		await expect(editOrgNodeSelect).toHaveValue(companyIDValue);

		const updateAssignmentResponse = page.waitForResponse((resp) => {
			return resp.request().method() === 'PATCH' && resp.url().includes('/org/assignments/');
		});
		await page.getByRole('button', { name: 'Save' }).click();
		expect((await updateAssignmentResponse).status()).toBe(200);
		await expect(page.locator('#org-assignments-timeline tbody tr')).toHaveCount(2);
		await expect(page.locator('#org-assignments-timeline')).toContainText(companyIDValue);
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

	test('管理员可创建 Position 并 Transfer 组织归属（DEV-PLAN-055）', async ({ page }) => {
		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		await page.goto('/org/nodes', { waitUntil: 'domcontentloaded' });
		await expect(page).toHaveURL(/\/org\/nodes/);
		await page.waitForFunction(() => typeof (window as any).htmx !== 'undefined', { timeout: 10_000 });

		const newNodeBtn = page.locator('[data-testid="org-new-node"]');
		await newNodeBtn.click();
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

		await tree.getByRole('button', { name: /Company/ }).click();
		await page.locator('[data-testid="org-new-child"]').click();
		await expect(page.getByText('Create node', { exact: true })).toBeVisible();
		await page.locator('input[name="code"]').fill('IT');
		await page.locator('input[name="name"]').fill('IT Team');
		await page.getByRole('button', { name: 'Create' }).click();
		await expect(tree.getByRole('button', { name: /IT Team/ })).toBeVisible();

		const itHxGet = await tree.getByRole('button', { name: /IT Team/ }).getAttribute('hx-get');
		expect(itHxGet).toBeTruthy();
		const itIDValue = new URL(itHxGet!, 'http://local').pathname.split('/').pop();
		expect(itIDValue).toBeTruthy();

		const hrHxGet = await tree.getByRole('button', { name: /HR Team/ }).getAttribute('hx-get');
		expect(hrHxGet).toBeTruthy();
		const hrIDValue = new URL(hrHxGet!, 'http://local').pathname.split('/').pop();
		expect(hrIDValue).toBeTruthy();

		await page.getByRole('link', { name: 'Positions' }).click();
		await expect(page).toHaveURL(/\/org\/positions/);

		const positionsTree = page.locator('#org-tree');
		const panelResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'GET' && resp.url().includes('/org/positions/panel');
		});
		await positionsTree.getByRole('button', { name: /HR Team/ }).click();
		expect((await panelResp).status()).toBe(200);
		await expect(page.locator('#org-positions-filters input[name="node_id"]')).toHaveValue(hrIDValue!);
		await page.waitForURL((url) => url.searchParams.get('node_id') === hrIDValue);
		expect(new URL(page.url()).searchParams.get('node_id')).toBe(hrIDValue);

			await page.getByRole('button', { name: 'Create position', exact: true }).click();
			await expect(page.locator('#org-position-details').getByText('Create position', { exact: true })).toBeVisible();

		await page.locator('input[name="code"]').fill('POS-001');
		await page.locator('input[name="title"]').fill('HR Specialist');
		const createResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'POST' && resp.url().includes('/org/positions');
		});
			await page.locator('#org-position-details').getByRole('button', { name: 'Create', exact: true }).click();
			expect((await createResp).status()).toBe(200);

			await expect(page.locator('#org-position-details')).toContainText('POS-001');
			await expect(page.locator('#org-positions-list')).toContainText('POS-001');
			await page.waitForURL(/position_id=/);
			const positionIDValue = new URL(page.url()).searchParams.get('position_id');
			expect(positionIDValue).toBeTruthy();

			const positionsBaseEffectiveDateStr = await page.locator('#effective-date').inputValue();
			expect(positionsBaseEffectiveDateStr).toMatch(/^\d{4}-\d{2}-\d{2}$/);
			const positionsFuture = addUTCDays(parseUTCDateString(positionsBaseEffectiveDateStr), 1);
			const positionsFutureStr = formatUTCDate(positionsFuture);
			await page.goto(
				`/org/positions?effective_date=${positionsFutureStr}&node_id=${hrIDValue}&position_id=${positionIDValue}`,
				{
					waitUntil: 'domcontentloaded',
				}
			);
			await page.waitForFunction(() => typeof (window as any).htmx !== 'undefined', { timeout: 10_000 });
			await expect(page.locator('#org-position-details')).toContainText('POS-001');

			await page.locator('#org-position-details').getByRole('button', { name: 'Edit', exact: true }).click();
			await expect(page.getByText('Edit position', { exact: true })).toBeVisible();

		const moveOrgNodeCombobox = page.locator('[data-testid="org-position-orgnode-combobox"]');
		await setComboboxValue({
			combobox: moveOrgNodeCombobox,
			query: 'IT Team',
			value: itIDValue!,
		});

		const updateResp = page.waitForResponse((resp) => {
			return resp.request().method() === 'PATCH' && resp.url().includes(`/org/positions/${positionIDValue}`);
		});
			await page.locator('#org-position-details').getByRole('button', { name: 'Save', exact: true }).click();
			expect((await updateResp).status()).toBe(200);

		await page.waitForURL(new RegExp(`node_id=${itIDValue}`));
		await expect(page.locator('#org-positions-filters input[name="node_id"]')).toHaveValue(itIDValue!);
		await expect(page.locator('#org-position-details')).toContainText('POS-001');
	});

	test('无 Org 权限账号访问 /org/positions 返回 Unauthorized（DEV-PLAN-055）', async ({ page }) => {
		await login(page, READONLY.email, READONLY.password);
		await assertAuthenticated(page);

		await page.goto('/org/positions', { waitUntil: 'domcontentloaded' });
		const response = await page.request.get('/org/positions', {
			headers: { Accept: 'application/json', 'X-Request-ID': 'e2e-org-positions-deny' },
		});
		expect([401, 403]).toContain(response.status());

		await expect(page.getByRole('heading', { name: /Permission required/i, level: 2 })).toBeVisible();
		await expect(page.locator('section[data-authz-container]')).toBeVisible();
	});
});
