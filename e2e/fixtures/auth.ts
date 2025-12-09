/**
 * Authentication fixtures for Playwright tests
 */

import { expect, Page } from '@playwright/test';

const SID_COOKIE_KEY = process.env.SID_COOKIE_KEY || 'sid';

export async function assertAuthenticated(page: Page) {
	const cookies = await page.context().cookies();
	const sidCookie = cookies.find(cookie => cookie.name === SID_COOKIE_KEY);
	if (!sidCookie) {
		const alertText = await page
			.locator('[role="alert"]')
			.first()
			.textContent()
			.catch(() => '');
		const hint = alertText ? `，页面提示：${alertText.trim()}` : '';
		throw new Error(`登录失败：未发现会话 cookie '${SID_COOKIE_KEY}'${hint}`);
	}
}

/**
 * Login helper function
 *
 * @param page - Playwright page object
 * @param email - User email
 * @param password - User password
 */
export async function login(page: Page, email: string, password: string) {
	await page.goto('/login');
	await page.getByLabel('Email').fill(email);
	await page.getByLabel('Password').fill(password);

	// Wait for navigation BEFORE clicking submit (Playwright best practice)
	// This prevents race conditions where navigation completes before waitForURL is called
	const submitButton = page.locator('form button[type="submit"]');
	await expect(submitButton).toHaveText(/log in/i);
	await Promise.all([
		page.waitForURL(url => !url.pathname.includes('/login'), {
			timeout: 15_000,
		}),
		submitButton.click(),
	]);

	// 确保跳转离开登录页且会话 cookie 已下发
	await page.waitForLoadState('networkidle');
	await page.waitForTimeout(300);
	await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 });
	await assertAuthenticated(page);
}

/**
 * Logout helper function
 *
 * @param page - Playwright page object
 */
export async function logout(page: Page) {
	await page.goto('/logout');
}

/**
 * Wait for Alpine.js initialization
 *
 * @param page - Playwright page object
 * @param timeout - Maximum wait time in ms (default: 5000)
 */
export async function waitForAlpine(page: Page, timeout: number = 5000) {
	// Wait for Alpine.js to be available on window
	await page.waitForFunction(
		() => {
			const win = window as any;
			return win.Alpine && win.Alpine.version;
		},
		{ timeout }
	).catch(() => {
		// Don't fail if Alpine isn't available, just continue
		console.warn('Alpine.js not detected within timeout, continuing anyway');
	});

	// Wait for body to be visible
	await page.waitForSelector('body', { state: 'visible' });

	// Allow time for initialization
	await page.waitForTimeout(1000);
}
