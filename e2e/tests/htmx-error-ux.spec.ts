import { test, expect, type Locator, type Page } from '@playwright/test';
import { assertAuthenticated, login } from '../fixtures/auth';
import {
	checkTestEndpointsHealth,
	resetTestDatabase,
	seedScenario,
} from '../fixtures/test-data';

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

async function createPerson(args: { page: Page; pernr: string; displayName: string }) {
	await args.page.goto('/person/persons/new', { waitUntil: 'domcontentloaded' });
	await expect(args.page).toHaveURL(/\/person\/persons\/new/);

	await args.page.locator('input[name="Pernr"]').fill(args.pernr);
	await args.page.locator('input[name="DisplayName"]').fill(args.displayName);

	await Promise.all([
		args.page.waitForURL(/\/person\/persons\/[0-9a-f-]+(?:\?.*)?$/),
		args.page.getByRole('button', { name: 'Create' }).click(),
	]);
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

function startOfUTCMonth(date: Date) {
	return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), 1));
}

function addUTCMonths(date: Date, months: number) {
	const out = new Date(date);
	out.setUTCMonth(out.getUTCMonth() + months);
	return out;
}

async function forceSelectValue(select: Locator, value: string, label: string) {
	await select.evaluate(
		(el, args) => {
			const select = el as HTMLSelectElement;
			const nextValue = String(args.value);
			const optionLabel = String(args.label);

			let option = Array.from(select.options).find(opt => opt.value === nextValue);
			if (!option) {
				option = new Option(optionLabel, nextValue, true, true);
				select.add(option);
			}

			select.value = nextValue;
			for (const opt of Array.from(select.options)) {
				opt.selected = opt.value === nextValue;
			}
			select.dispatchEvent(new Event('change', { bubbles: true }));
		},
		{ value, label }
	);
}

test.describe('HTMX Error UX (DEV-PLAN-043)', () => {
	test.beforeEach(async ({ request }) => {
		await ensureSeeded({ request });
	});

	test('422 + text/html 可 swap 表单错误片段（不弹默认 toast）', async ({ page }) => {
		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		await page.goto('/org/assignments', { waitUntil: 'domcontentloaded' });
		await expect(page).toHaveURL(/\/org\/assignments/);
		await page.waitForFunction(() => typeof (window as any).htmx !== 'undefined', { timeout: 10_000 });

		const createResp = page.waitForResponse(
			resp => resp.request().method() === 'POST' && resp.url().includes('/org/assignments'),
			{ timeout: 30_000 }
		);
		await page
			.locator('#org-assignment-form')
			.getByRole('button', { name: 'Create', exact: true })
			.click();
		expect((await createResp).status()).toBe(422);

		await expect(page.locator('#org-assignment-form [data-testid="field-error"]')).toBeVisible();
	});

	test('409 freeze cutoff 不再静默失败（可 swap 错误片段）', async ({ page }) => {
		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		await createPerson({ page, pernr: '0001', displayName: 'E2E Person 0001' });

		await page.goto('/org/assignments', { waitUntil: 'domcontentloaded' });
		await expect(page).toHaveURL(/\/org\/assignments/);
		await page.waitForFunction(() => typeof (window as any).htmx !== 'undefined', { timeout: 10_000 });

		const baseEffectiveDateStr = await page.locator('#effective-date').inputValue();
		expect(baseEffectiveDateStr).toMatch(/^\d{4}-\d{2}-\d{2}$/);
		const baseEffectiveDate = parseUTCDateString(baseEffectiveDateStr);
		const invalidEffectiveDate = addUTCMonths(startOfUTCMonth(baseEffectiveDate), -2);
		const invalidEffectiveDateStr = formatUTCDate(invalidEffectiveDate);

		const effectiveDateChangeResp = page.waitForResponse(resp => {
			return (
				resp.request().method() === 'GET' &&
				resp.url().includes('/org/assignments') &&
				resp.url().includes(`effective_date=${invalidEffectiveDateStr}`)
			);
		});
		const effectiveDateInput = page.locator('#effective-date');
		await effectiveDateInput.fill(invalidEffectiveDateStr);
		await effectiveDateInput.dispatchEvent('change');
		expect((await effectiveDateChangeResp).status()).toBe(200);
		await expect(page.locator('#org-assignment-form form')).toHaveAttribute(
			'hx-post',
			new RegExp(`effective_date=${invalidEffectiveDateStr}`)
		);

		await page.getByLabel('Pernr').fill('0001');
		await expect(page.locator('#org-pernr')).toHaveValue('0001');

		const orgNodeSelect = page.locator(
			'[data-testid="org-assignment-orgnode-combobox"] select[name="org_node_id"]'
		);
		await forceSelectValue(orgNodeSelect, '00000000-0000-0000-0000-000000000000', 'Dummy');

		const createResp = page.waitForResponse(
			resp => resp.request().method() === 'POST' && resp.url().includes('/org/assignments'),
			{ timeout: 30_000 }
		);
		await page
			.locator('#org-assignment-form')
			.getByRole('button', { name: 'Create', exact: true })
			.click();
		expect((await createResp).status()).toBe(409);

		await expect(page.locator('#org-assignment-form div[class*="bg-red-500/10"]')).toBeVisible();
	});

	test('403 + text/html 允许 swap（Authz forbidden partial）', async ({ page }) => {
		await login(page, READONLY.email, READONLY.password);
		await assertAuthenticated(page);

		await page.goto('/_dev', { waitUntil: 'domcontentloaded' });
		await page.waitForFunction(() => typeof (window as any).htmx !== 'undefined', { timeout: 10_000 });

		expect(await page.getByRole('heading', { name: /Permission required/i }).count()).toBe(0);

		const forbiddenResp = page.waitForResponse(resp => {
			const hxRequest = resp.request().headers()['hx-request'];
			return resp.request().method() === 'GET' && resp.url().includes('/org/nodes') && hxRequest === 'true';
		});

		await page.evaluate(() => {
			const htmx = (window as any).htmx;
			htmx.ajax('GET', '/org/nodes', { target: 'body', swap: 'innerHTML' });
		});

		expect((await forbiddenResp).status()).toBe(403);
		await expect(page.getByRole('heading', { name: /Permission required/i })).toBeVisible();
	});

	test('500 + JSON 显示兜底 toast（无 HX-Trigger）', async ({ page }) => {
		await login(page, ADMIN.email, ADMIN.password);
		await assertAuthenticated(page);

		await page.goto('/_dev', { waitUntil: 'domcontentloaded' });
		await page.waitForFunction(() => typeof (window as any).htmx !== 'undefined', { timeout: 10_000 });

		const errResp = page.waitForResponse(resp => {
			const hxRequest = resp.request().headers()['hx-request'];
			return (
				resp.request().method() === 'GET' &&
				resp.url().includes('/__test__/http_error') &&
				hxRequest === 'true'
			);
		});

		await page.evaluate(() => {
			const htmx = (window as any).htmx;
			htmx.ajax(
				'GET',
				'/__test__/http_error?status=500&format=json&code=E2E_TEST_INTERNAL&message=Something%20broke',
				{ target: 'body', swap: 'innerHTML' }
			);
		});

		expect((await errResp).status()).toBe(500);

		const toastContainer = page.locator('div[aria-live="assertive"]');
		const alertToast = toastContainer.getByRole('alert');
		await expect(alertToast).toContainText('E2E_TEST_INTERNAL');
		await expect(alertToast).toContainText('Something broke');
	});
});
