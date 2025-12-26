import { test, expect } from '@playwright/test';
import { login, logout, waitForAlpine, resetTestDatabase, seedScenario } from '../../fixtures';

test.describe.configure({ mode: 'serial' });

test.describe('authz policies apply', () => {
	test.beforeAll(async ({ request }) => {
		await resetTestDatabase(request, { reseedMinimal: false });
		await seedScenario(request, 'comprehensive');
	});

	test.afterEach(async ({ page }) => {
		await logout(page);
	});

	test('can apply and rollback direct policy changes', async ({ page }) => {
		let currentRevision = '';
		let policySubject = '';
		let policyDomain = '';
		await login(page, 'test@gmail.com', 'TestPass123!');
		await waitForAlpine(page);

		await page.goto('/users?Search=noperson@example.com', { waitUntil: 'domcontentloaded' });
		await expect(page.locator('#users-table-body')).toBeVisible();

		const userEditLink = page.locator('#users-table-body a[href^="/users/"]').first();
		await expect(userEditLink).toBeVisible();
		await userEditLink.click();
		await expect(page).toHaveURL(/\/users\/[0-9]+$/);
		const userURL = page.url();

			await page.locator('button', { hasText: /permissions/i }).first().click();
			await expect(page.locator('#user-policy-board')).toBeVisible();

		const addMenuDetails = page.locator('details:has([data-testid="authz-user-stage-menu"])').first();
		await addMenuDetails.evaluate(el => {
			(el as unknown as { open: boolean }).open = true;
		});
		await page.getByTestId('authz-user-stage-open-direct').click();

			const stageDialog = page.locator('#stage-policy-direct dialog');
			await expect(stageDialog).toBeVisible();

		await stageDialog.locator('input[name="object"]').fill('logging.logs');
		await stageDialog.locator('input[name="action"]').fill('view');

			await page.getByTestId('authz-user-stage-save-direct').click();
			const workspace = page.locator('#authz-workspace');
			await expect(workspace).toHaveCount(1, { timeout: 15_000 });
			const baseRevision = await workspace.locator('input[name="base_revision"]').inputValue();
			policySubject = await workspace.locator('input[name="subject"]').inputValue();
			policyDomain = await workspace.locator('input[name="domain"]').inputValue();
			expect(policySubject).not.toBe('');
			expect(policyDomain).not.toBe('');

			const applyResp = await page.request.post('/core/api/authz/policies/apply', {
				data: {
					base_revision: baseRevision,
					subject: policySubject,
					domain: policyDomain,
					reason: 'e2e apply direct policy',
					changes: [
						{
							stage_kind: 'add',
							type: 'p',
							subject: policySubject,
							domain: policyDomain,
							object: 'logging.logs',
							action: 'view',
							effect: 'allow',
						},
					],
				},
			});
			expect(applyResp.ok()).toBeTruthy();
			const applyData = await applyResp.json();
			currentRevision = String(applyData?.revision || '');
			expect(currentRevision).not.toBe('');
			await page.reload({ waitUntil: 'domcontentloaded' });
			await expect(page.locator('#authz-workspace')).toHaveCount(0, { timeout: 15_000 });

		const appliedScreenshot = test.info().outputPath('authz-apply-admin.png');
		await page.screenshot({ path: appliedScreenshot, fullPage: true });
		test.info().attach('authz-apply-admin.png', { path: appliedScreenshot, contentType: 'image/png' });

		await logout(page);
		await login(page, 'noperson@example.com', 'TestPass123!');
		await page.goto('/logs', { waitUntil: 'domcontentloaded' });
		await expect(page.getByRole('heading', { level: 1 })).toContainText(/Logs/i);
		await expect(page.locator('[data-policy-inspector]')).toBeVisible();

		const allowedScreenshot = test.info().outputPath('authz-apply-allowed-user.png');
		await page.screenshot({ path: allowedScreenshot, fullPage: true });
		test.info().attach('authz-apply-allowed-user.png', { path: allowedScreenshot, contentType: 'image/png' });

		await logout(page);
		await login(page, 'test@gmail.com', 'TestPass123!');
		await waitForAlpine(page);

			await page.goto(userURL, { waitUntil: 'domcontentloaded' });
			await page.locator('button', { hasText: /permissions/i }).first().click();
			await expect(page.locator('#user-policy-board')).toBeVisible();

			const directColumn = page.locator('#user-policy-direct');
			const ruleRow = directColumn.locator('tr', { hasText: 'logging.logs' }).filter({ hasText: 'view' }).first();
			await expect(ruleRow).toBeVisible();

			const rollbackResp = await page.request.post('/core/api/authz/policies/apply', {
				data: {
					base_revision: currentRevision,
					subject: policySubject,
					domain: policyDomain,
					reason: 'e2e rollback direct policy',
					changes: [
						{
							stage_kind: 'remove',
							type: 'p',
							subject: policySubject,
							domain: policyDomain,
							object: 'logging.logs',
							action: 'view',
							effect: 'allow',
						},
					],
				},
			});
			expect(rollbackResp.ok()).toBeTruthy();
			await page.reload({ waitUntil: 'domcontentloaded' });
			await expect(page.locator('#authz-workspace')).toHaveCount(0, { timeout: 15_000 });
		});
	});
