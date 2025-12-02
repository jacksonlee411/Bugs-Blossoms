import { test, expect } from '@playwright/test';
import type { Page } from '@playwright/test';
import { login, logout } from '../../fixtures/auth';
import { resetTestDatabase, seedScenario } from '../../fixtures/test-data';

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
	languageCode: 'ru',
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
	await page.goto('/users');
	await expect(page).not.toHaveURL(/\/login/, { timeout: 5_000 });
	await expect(page).toHaveURL(/\/users/);
}

async function openNewUserForm(page: Page) {
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
	await page.locator('[name=FirstName]').fill(data.firstName);
	await page.locator('[name=LastName]').fill(data.lastName);
	await page.locator('[name=MiddleName]').fill(data.middleName);
	await page.locator('[name=Email]').fill(data.email);
	await page.locator('[name=Phone]').fill(data.phone);
	if (data.password) {
		await page.locator('[name=Password]').fill(data.password);
	}
	if (data.languageCode) {
		await page.locator('[name=Language]').selectOption(data.languageCode);
	}
}

async function createUserThroughUI(page: Page, data: UserFormData) {
	await goToUsersPage(page);
	await openNewUserForm(page);
	await fillUserForm(page, data);
	await selectFirstRole(page);
	await page.locator('[id=save-btn]').click();
	await page.waitForURL(/\/users$/);
}

function getUserRowLocator(page: Page, data: UserFormData) {
	return page.locator('tbody tr').filter({ hasText: `${data.firstName} ${data.lastName}` });
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
		const userRow = getUserRowLocator(page, EDIT_USER);
		await expect(userRow).toHaveCount(1);
		await userRow.locator('td a').click();

		await expect(page).toHaveURL(/\/users\/.+/);

		// Edit the user details
		await fillUserForm(page, UPDATED_EDIT_USER);
		await page.locator('[id=save-btn]').click();

		// Wait for redirect after save
		await page.waitForURL(/\/users$/);

		// Verify changes in the users list
		const usersTable = page.locator('#users-table-body');
		await expect(usersTable).toContainText(
			`${UPDATED_EDIT_USER.firstName} ${UPDATED_EDIT_USER.lastName}`,
		);

		// Verify phone number persists by checking the edit page
		const updatedUserRow = getUserRowLocator(page, UPDATED_EDIT_USER);
		await updatedUserRow.locator('td a').click();
		await expect(page).toHaveURL(/\/users\/.+/);
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
