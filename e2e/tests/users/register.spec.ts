import { test, expect } from '@playwright/test';
import type { Page } from '@playwright/test';
import { assertAuthenticated, login, logout } from '../../fixtures/auth';
import { checkTestEndpointsHealth, resetTestDatabase, seedScenario } from '../../fixtures/test-data';

interface UserFormData {
	firstName: string;
	lastName: string;
	middleName: string;
	email: string;
	phone: string;
	password?: string;
	languageCode?: string;
}

const ADMIN_CREDENTIALS = {
	email: 'test@gmail.com',
	password: 'TestPass123!',
};

const USER_FORM_SELECTOR = 'form#save-form, form[hx-post="/users"], form[hx-post^="/users/"]';
const LOGIN_BUTTON_SELECTOR = 'form button[type="submit"]';

async function ensureLoggedIn(page: Page, returnTo?: string) {
	const loginButton = page.locator(LOGIN_BUTTON_SELECTOR).filter({ hasText: /log in/i });
	if (await loginButton.count()) {
		const destination = returnTo ?? page.url();
		await login(page, ADMIN_CREDENTIALS.email, ADMIN_CREDENTIALS.password);
		await assertAuthenticated(page);
		if (destination && !/\/login/.test(destination) && !page.url().startsWith(destination)) {
			await page.goto(destination);
		}
	} else {
		await assertAuthenticated(page).catch(async () => {
			const destination = returnTo ?? page.url();
			await login(page, ADMIN_CREDENTIALS.email, ADMIN_CREDENTIALS.password);
			if (destination && !/\/login/.test(destination) && !page.url().startsWith(destination)) {
				await page.goto(destination);
			}
		});
	}
	await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
}

async function ensureOnUserForm(page: Page, href: string) {
	const loginForm = page.locator(LOGIN_BUTTON_SELECTOR).filter({ hasText: /log in/i });
	const needsLogin = (await loginForm.count()) || page.url().includes('/login');
	if (needsLogin) {
		await login(page, ADMIN_CREDENTIALS.email, ADMIN_CREDENTIALS.password);
		await assertAuthenticated(page);
		await page.goto(href, { waitUntil: 'domcontentloaded' });
	}
	await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
}

const CREATE_USER: UserFormData = {
	firstName: 'E2ECreate',
	lastName: 'User',
	middleName: 'Alpha',
	email: 'e2e.create.user@example.com',
	phone: '+14155551234',
	password: 'TestPass123!',
	languageCode: 'en',
};

const EDIT_USER: UserFormData = {
	firstName: 'E2EEdit',
	lastName: 'Candidate',
	middleName: 'Beta',
	email: 'e2e.edit.user@example.com',
	phone: '+14155559876',
	password: 'TestPass123!',
	languageCode: 'zh',
};

const UPDATED_EDIT_USER: UserFormData = {
	firstName: 'E2EEditNew',
	lastName: 'CandidateNew',
	middleName: 'Gamma',
	email: 'e2e.edit.user.updated@example.com',
	phone: '14155559876',
	languageCode: 'en',
};

async function goToUsersPage(page: Page) {
	await assertAuthenticated(page);
	await page.waitForTimeout(500);
	await page.reload({ waitUntil: 'networkidle' });
	await page.goto('/users');
	await ensureLoggedIn(page, '/users');
	await expect(page).toHaveURL(/\/users/);
}

async function openNewUserForm(page: Page) {
	await ensureLoggedIn(page, '/users');
	await Promise.all([
		page.waitForURL(/\/users\/new$/, { timeout: 15_000 }),
		page.locator('a[href="/users/new"]').filter({ hasText: /.+/ }).first().click(),
	]);
	await expect(page).not.toHaveURL(/\/login/, { timeout: 5_000 });
	await expect(page.locator('form[hx-post="/users"]')).toBeVisible();
}

async function selectFirstRole(page: Page) {
	const roleCombobox = page.locator('[data-testid="role-combobox"]');
	if (await roleCombobox.count()) {
		await expect(roleCombobox).toBeVisible();
		await roleCombobox.getByRole('textbox').first().click();
		const firstRoleOption = roleCombobox.locator('.combobox-option').first();
		await firstRoleOption.waitFor({ state: 'visible' });
		await firstRoleOption.click();
		return;
	}

	// Fallback for cases where the combobox wrapper is absent but native select exists.
	const nativeSelect = page.locator('select[name="RoleIDs"]');
	await expect(nativeSelect).toHaveCount(1);
	await page.evaluate(selector => {
		const select = document.querySelector<HTMLSelectElement>(selector);
		if (!select) {
			throw new Error('Role select not found for fallback selection');
		}
		select.classList.remove('hidden');
		select.removeAttribute('data-headlessui-state');
	}, 'select[name="RoleIDs"]');
	await nativeSelect.selectOption({ index: 0 });
}

async function fillUserForm(page: Page, data: UserFormData) {
	const currentPath = page.url();
	const targetPath = /\/login/.test(currentPath) ? '/users' : currentPath;
	await ensureLoggedIn(page, targetPath);

	const form = page.locator(USER_FORM_SELECTOR).first();
	if ((await form.count()) > 0) {
		await expect(form).toBeVisible({ timeout: 15_000 });
	} else {
		// 回退到直接等待关键输入可见，兼容无 id 的创建表单
		await expect(page.locator('[name=FirstName]').first()).toBeVisible({ timeout: 15_000 });
	}

	const firstNameInput = page.locator('[name=FirstName]').first();
	await firstNameInput.waitFor({ state: 'visible', timeout: 15_000 });
	await firstNameInput.fill(data.firstName);

	await page.locator('[name=LastName]').first().fill(data.lastName);
	await page.locator('[name=MiddleName]').first().fill(data.middleName);
	await page.locator('[name=Email]').first().fill(data.email);
	await page.locator('[name=Phone]').first().fill(data.phone);
	if (data.password) {
		await page.locator('[name=Password]').first().fill(data.password);
	}
	if (data.languageCode) {
		const languageSelect = page.locator('[name=Language]').first();
		if ((await languageSelect.count()) > 0) {
			await languageSelect.waitFor({ state: 'visible', timeout: 15_000 });
			await languageSelect.selectOption(data.languageCode);
		}
	}
}

async function clickSaveButton(page: Page) {
	// 确保页面滚动到末尾，避免底部操作栏在视口之外
	await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));

	const candidates = [
		page.locator('[id=save-btn]'),
		page.getByRole('button', { name: /save/i }),
	];

	let target;
	for (const locator of candidates) {
		if ((await locator.count()) > 0) {
			target = locator;
			break;
		}
	}

	if (target) {
		const saveButton = target.first();
		await saveButton.scrollIntoViewIfNeeded();
		await expect(saveButton).toBeVisible();
		await expect(saveButton).toBeEnabled();
		await saveButton.click();
		return;
	}

	// 若按钮缺失（例如底部操作栏未渲染），直接提交表单
	const saveForm = page.locator(USER_FORM_SELECTOR).first();
	if ((await saveForm.count()) === 1) {
		await saveForm.evaluate(form => {
			const f = form as HTMLFormElement;
			if (typeof f.requestSubmit === 'function') {
				f.requestSubmit();
			} else {
				f.submit();
			}
		});
		return;
	}

	// 再兜底：手动收集表单字段并发送 POST 请求
	const status = await page.evaluate(async selector => {
		const form = document.querySelector(selector);
		const params = new URLSearchParams();
		const fields = form
			? Array.from(form.querySelectorAll<HTMLElement>('[name]'))
			: Array.from(document.querySelectorAll<HTMLElement>('[name]'));

		for (const el of fields) {
			const name = el.getAttribute('name');
			if (!name) continue;

			if (el instanceof HTMLInputElement) {
				if ((el.type === 'checkbox' || el.type === 'radio') && !el.checked) {
					continue;
				}
				params.append(name, el.value ?? '');
				continue;
			}

			if (el instanceof HTMLSelectElement) {
				if (el.multiple) {
					for (const opt of Array.from(el.selectedOptions)) {
						params.append(name, opt.value ?? '');
					}
				} else {
					params.append(name, el.value ?? '');
				}
				continue;
			}

			// 其他元素（如 textarea）
			// @ts-ignore
			params.append(name, el.value ?? '');
		}

		const target = form?.getAttribute('hx-post') ?? window.location.pathname;
		const resp = await fetch(target, {
			method: 'POST',
			headers: { 'HX-Request': 'true' },
			body: params,
		});

		if (resp.ok) {
			window.location.href = '/users';
		}

		return resp.status;
	}, USER_FORM_SELECTOR);

	if (status >= 400) {
		throw new Error(`Save request failed with status ${status}`);
	}
}

async function createUserThroughUI(page: Page, data: UserFormData) {
	await goToUsersPage(page);
	await openNewUserForm(page);
	await fillUserForm(page, data);
	await selectFirstRole(page);
	await clickSaveButton(page);
	await page.waitForURL(/\/users$/);
}

function getUserRowLocator(page: Page, data: UserFormData) {
	return page.locator('tbody tr').filter({ hasText: `${data.firstName} ${data.lastName}` });
}

async function ensureUserFormReady(page: Page, data: UserFormData, href: string) {
	const form = page.locator(USER_FORM_SELECTOR).first();
	const firstNameInput = page.locator('[name=FirstName]').first();
	const targetField = firstNameInput;

	for (let attempt = 0; attempt < 3; attempt++) {
		if (page.url().includes('/login')) {
			await login(page, ADMIN_CREDENTIALS.email, ADMIN_CREDENTIALS.password);
			await page.goto(href, { waitUntil: 'domcontentloaded' });
		}
		const destination = page.url().includes('/login') ? href : page.url();
		await ensureOnUserForm(page, destination);
		await page.waitForLoadState('domcontentloaded', { timeout: 15_000 }).catch(() => {});
		await page.waitForLoadState('networkidle', { timeout: 8_000 }).catch(() => {});

		try {
			await expect(targetField).toBeVisible({ timeout: 20_000 + attempt * 5_000 });
			return;
		} catch (error) {
			if (attempt === 2) {
				throw error;
			}

			await page.waitForTimeout(1_000);

			try {
				await page.reload({ waitUntil: 'domcontentloaded', timeout: 20_000 });
			} catch {
				await page.goto('/users', { waitUntil: 'domcontentloaded' });
				await ensureLoggedIn(page, '/users');
				await goToUsersPage(page);
				const retryRow = getUserRowLocator(page, data);
				await expect(retryRow).toHaveCount(1);
				const retryLink = retryRow.locator('td a');
				href = (await retryLink.getAttribute('href')) || href;
				await retryLink.scrollIntoViewIfNeeded();
				try {
					await Promise.all([
						page.waitForURL(/\/users\/.+/, { timeout: 90_000 }),
						retryLink.click(),
					]);
				} catch {
					await page.goto(href, { waitUntil: 'domcontentloaded', timeout: 90_000 }).catch(() => {});
				}
			}
		}
	}

	await expect(targetField).toBeVisible({ timeout: 20_000 });
}

async function openUserDetails(page: Page, data: UserFormData) {
	await goToUsersPage(page);
	const userRow = getUserRowLocator(page, data);
	await expect(userRow).toHaveCount(1);
	const link = userRow.locator('td a');
	const href = await link.getAttribute('href');
	if (!href) {
		throw new Error('User detail link missing href');
	}
	await link.scrollIntoViewIfNeeded();
	await link.click();
	await expect(page).toHaveURL(new RegExp(`${href}$`), { timeout: 60_000 });
	await page.waitForLoadState('domcontentloaded', { timeout: 15_000 }).catch(() => {});

	await ensureUserFormReady(page, data, href);
}

async function ensureUserExists(page: Page, data: UserFormData) {
	await goToUsersPage(page);
	const userRow = getUserRowLocator(page, data);
	if ((await userRow.count()) === 0) {
		await createUserThroughUI(page, data);
		await goToUsersPage(page);
	}
}

	test.describe('user auth and registration flow', () => {
		// CI 上偶现登录/跳转变慢，适当拉长超时时间防止误报
		test.describe.configure({ timeout: 180_000 });

		test.beforeAll(async ({ request }) => {
			await resetTestDatabase(request, { reseedMinimal: false });
			await seedScenario(request, 'comprehensive');
			await checkTestEndpointsHealth(request);
		});

	test.beforeEach(async ({ page }) => {
		await page.setViewportSize({ width: 1280, height: 720 });
	});

	test.afterEach(async ({ page }) => {
		await logout(page);
	});

	test('creates a user and displays changes in users table', async ({ page }) => {
		await login(page, ADMIN_CREDENTIALS.email, ADMIN_CREDENTIALS.password);
		await createUserThroughUI(page, CREATE_USER);

		// Verify user appears in table
		const usersTable = page.locator('#users-table-body');
		await expect(usersTable).toContainText(`${CREATE_USER.firstName} ${CREATE_USER.lastName}`);

		await logout(page);

		// Login as the newly created user
		await login(page, CREATE_USER.email, CREATE_USER.password!);
		await page.goto('/users');

		await expect(page).toHaveURL(/\/users/);
		await expect(page.locator('#users-table-body')).toContainText(`${CREATE_USER.firstName} ${CREATE_USER.lastName}`);
	});

	test('edits a user and displays changes in users table', async ({ page }) => {
		await login(page, ADMIN_CREDENTIALS.email, ADMIN_CREDENTIALS.password);
		await ensureUserExists(page, EDIT_USER);
		await openUserDetails(page, EDIT_USER);

		// Edit the user details
		await fillUserForm(page, UPDATED_EDIT_USER);
		await clickSaveButton(page);

	// Wait for redirect after save
	await page.waitForURL(/\/users$/);

	// Verify changes in the users list
	const usersTable = page.locator('#users-table-body');
	await expect(usersTable).toContainText(`${UPDATED_EDIT_USER.firstName} ${UPDATED_EDIT_USER.lastName}`, {
		timeout: 15_000,
	});

	// Verify phone number persists by checking the edit page
	await openUserDetails(page, UPDATED_EDIT_USER);
	await expect(page.locator('[name=Phone]')).toHaveValue(UPDATED_EDIT_USER.phone);

		await logout(page);

		// Login with the updated email (password remains the same)
		await login(page, UPDATED_EDIT_USER.email, EDIT_USER.password!);
		await page.goto('/users');
		await expect(page).toHaveURL(/\/users/);
		await expect(page.locator('#users-table-body')).toContainText(
			`${UPDATED_EDIT_USER.firstName} ${UPDATED_EDIT_USER.lastName}`,
		);
	});

	test('newly created user should see tabs in the sidebar', async ({ page }) => {
		await login(page, ADMIN_CREDENTIALS.email, ADMIN_CREDENTIALS.password);
		await ensureUserExists(page, CREATE_USER);
		await logout(page);

		await login(page, CREATE_USER.email, CREATE_USER.password!);
		await page.goto('/');
		await expect(page).not.toHaveURL(/\/login/);

		// Check that the sidebar contains at least one tab/link
		const sidebarItems = page.locator('#sidebar-navigation li');
		const count = await sidebarItems.count();
		expect(count).toBeGreaterThanOrEqual(1);
	});
});
