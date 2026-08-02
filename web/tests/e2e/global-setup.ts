import { execSync } from 'child_process';
import path from 'path';

const REPO_ROOT = path.resolve(import.meta.dirname, '../../..');

/**
 * Global setup for Playwright E2E tests.
 *
 * When `CURLEW_MANAGE_STACK=1` is set, starts the docker-compose test stack
 * via `scripts/test-stack.sh up`. Opt-in so local developers can point
 * Playwright at an already-running stack without rebuilding containers.
 */
export default async function globalSetup(): Promise<void> {
	if (process.env.CURLEW_MANAGE_STACK === '1') {
		console.log('[global-setup] Starting test stack via scripts/test-stack.sh...');
		const script = path.join(REPO_ROOT, 'scripts', 'test-stack.sh');
		try {
			execSync(`bash ${script} up`, {
				encoding: 'utf8',
				stdio: 'inherit',
				timeout: 300_000 // 5 minutes
			});
		} catch (err) {
			console.error('[global-setup] Failed to start test stack:', err);
			throw err;
		}
	}
}
