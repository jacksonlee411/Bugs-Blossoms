import { test, expect, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { login, logout } from '../../fixtures/auth';
import { resetTestDatabase, seedScenario } from '../../fixtures/test-data';
import * as fs from 'fs';

type AxeImpact = 'minor' | 'moderate' | 'serious' | 'critical' | undefined;

const viewports = [
	{ name: 'mobile', width: 390, height: 844 },
	{ name: 'laptop', width: 1280, height: 720 },
	{ name: 'desktop', width: 1728, height: 1117 },
] as const;

function countViolationsByImpact(impacts: AxeImpact[]) {
	return impacts.reduce(
		(acc, impact) => {
			const key = impact ?? 'unknown';
			acc[key] = (acc[key] ?? 0) + 1;
			return acc;
		},
		{} as Record<string, number>,
	);
}

async function runAxeSmoke(pageName: string, page: Page) {
	const results = await new AxeBuilder({ page }).analyze();
	const counts = countViolationsByImpact(results.violations.map((v) => v.impact));

	const outputPath = test.info().outputPath(`axe-${pageName}.json`);
	fs.writeFileSync(outputPath, JSON.stringify(results, null, 2));
	test.info().attach(`axe-${pageName}.json`, {
		path: outputPath,
		contentType: 'application/json',
	});

	const critical = counts.critical ?? 0;
	const serious = counts.serious ?? 0;
	const moderate = counts.moderate ?? 0;
	const minor = counts.minor ?? 0;
	const unknown = counts.unknown ?? 0;

	console.log(
		`[axe] ${pageName}: critical=${critical}, serious=${serious}, moderate=${moderate}, minor=${minor}, unknown=${unknown}`,
	);
	for (const v of results.violations) {
		console.log(`[axe] violation: id=${v.id}, impact=${v.impact}, help=${v.help}, nodes=${v.nodes.length}`);
		for (const node of v.nodes.slice(0, 3)) {
			const target = Array.isArray(node.target) ? node.target.join(', ') : String(node.target);
			console.log(`[axe]  - target: ${target}`);
			if (node.failureSummary) {
				console.log(`[axe]    ${node.failureSummary.replaceAll('\n', ' ')}`);
			}
		}
		if (v.nodes.length > 3) {
			console.log(`[axe]  - ... ${v.nodes.length - 3} more nodes`);
		}
	}

	expect(critical, `${pageName} should have 0 critical axe violations`).toBe(0);
	expect(serious, `${pageName} should have 0 serious axe violations`).toBe(0);

	return { critical, serious, moderate, minor, unknown };
}

test.describe('a11y smoke - authz ui', () => {
	test.describe.configure({ mode: 'serial' });

	test.beforeAll(async ({ request }) => {
		await resetTestDatabase(request, { reseedMinimal: false });
		await seedScenario(request, 'comprehensive');
	});

	test.afterEach(async ({ page }) => {
		await logout(page);
	});

	test('authz pages render across common viewports', async ({ page }) => {
		await login(page, 'test@gmail.com', 'TestPass123!');

		for (const vp of viewports) {
			await page.setViewportSize({ width: vp.width, height: vp.height });

			await page.goto('/roles', { waitUntil: 'domcontentloaded' });
			const firstRolePoliciesLink = page.locator('a[href^="/roles/"][href$="/policies"]').first();
			if (await firstRolePoliciesLink.count()) {
				await firstRolePoliciesLink.click();
				await expect(page).toHaveURL(/\/roles\/[0-9]+\/policies/);
				await expect(page.locator('#policy-matrix')).toBeVisible();
				await expect(page.locator('[data-testid="authz-role-stage-open"]')).toBeVisible();
			}

			await page.goto('/users', { waitUntil: 'domcontentloaded' });
			const firstUserPoliciesLink = page.locator('a[href^="/users/"][href$="/policies"]').first();
			if (await firstUserPoliciesLink.count()) {
				await firstUserPoliciesLink.click();
				await expect(page).toHaveURL(/\/users\/[0-9]+\/policies/);
				await expect(page.locator('#user-policy-board')).toBeVisible();
				await expect(page.locator('[data-testid="authz-user-stage-menu"]')).toBeVisible();
			}
		}
	});

	test('core authz pages have no critical/serious violations', async ({ page }) => {
		await login(page, 'test@gmail.com', 'TestPass123!');

		const results: Array<{
			page: string;
			critical: number;
			serious: number;
			moderate: number;
			minor: number;
			unknown: number;
		}> = [];

		await page.goto('/roles', { waitUntil: 'domcontentloaded' });
		const firstRolePoliciesLink = page.locator('a[href^="/roles/"][href$="/policies"]').first();
		if (await firstRolePoliciesLink.count()) {
			await firstRolePoliciesLink.click();
			await expect(page).toHaveURL(/\/roles\/[0-9]+\/policies/);
			results.push({ page: 'roles-policy-matrix', ...(await runAxeSmoke('roles-policy-matrix', page)) });
		}

		await page.goto('/users', { waitUntil: 'domcontentloaded' });
		const firstUserPoliciesLink = page.locator('a[href^="/users/"][href$="/policies"]').first();
		if (await firstUserPoliciesLink.count()) {
			await firstUserPoliciesLink.click();
			await expect(page).toHaveURL(/\/users\/[0-9]+\/policies/);
			results.push({ page: 'users-policy-board', ...(await runAxeSmoke('users-policy-board', page)) });
		}

		test.info().attach('axe-summary.json', {
			body: JSON.stringify(results, null, 2),
			contentType: 'application/json',
		});
	});

	test('unauthorized page has no critical/serious violations', async ({ page }) => {
		await login(page, 'noperson@example.com', 'TestPass123!');

		await page.goto('/person/persons', { waitUntil: 'domcontentloaded' });
		await expect(page.locator('section[data-authz-container]')).toBeVisible();

		await runAxeSmoke('unauthorized-person-persons', page);
	});
});
