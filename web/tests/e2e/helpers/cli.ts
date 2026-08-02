import { execSync, type ExecSyncOptionsWithStringEncoding } from 'child_process';
import path from 'path';

const REPO_ROOT = path.resolve(import.meta.dirname, '../../../..');
const CLI_BIN = path.join(REPO_ROOT, 'curlew');

export interface RunCurlewOptions {
	/** Additional CLI flags (e.g. ['--report-upload', '--org', 'acme']) */
	flags?: string[];
	/** Path to the collection YAML file (relative to repo root or absolute) */
	collection: string;
	/** Expected exit code; defaults to 0. Throws if actual code differs. */
	expectExit?: number;
	/** Extra environment variables */
	env?: Record<string, string>;
}

export interface RunCurlewResult {
	stdout: string;
	stderr: string;
	exitCode: number;
	/** The res_... result id parsed from stdout (if present) */
	resultId?: string;
	/** Number of passing assertions parsed from stdout */
	pass?: number;
	/** Number of failing assertions parsed from stdout */
	fail?: number;
}

/**
 * Runs the curlew binary with the given flags and collection.
 * Returns parsed stdout/stderr and the result id.
 * Throws if the exit code doesn't match `expectExit` (default 0).
 */
export function runCurlew(opts: RunCurlewOptions): RunCurlewResult {
	const { flags = [], collection, expectExit = 0, env = {} } = opts;
	const importsBackendUrl = flags.some(
		(flag, index) => flag === '--env-var' && flags[index + 1]?.startsWith('CURLEW_BACKEND_URL')
	);
	const effectiveFlags = env.CURLEW_BACKEND_URL && !importsBackendUrl
		? ['--env-var', 'CURLEW_BACKEND_URL', ...flags]
		: flags;

	const collectionPath = path.isAbsolute(collection)
		? collection
		: path.join(REPO_ROOT, collection);

	const args = [collectionPath, ...effectiveFlags];
	const cmd = `${CLI_BIN} run ${args.map((a) => `"${a}"`).join(' ')}`;

	const execOpts: ExecSyncOptionsWithStringEncoding = {
		encoding: 'utf8',
		cwd: REPO_ROOT,
		env: { ...process.env, ...env },
		// Don't throw on non-zero exit — we check it ourselves
		stdio: ['ignore', 'pipe', 'pipe']
	};

	let stdout = '';
	let stderr = '';
	let exitCode = 0;

	try {
		stdout = execSync(cmd, execOpts);
	} catch (err: unknown) {
		// execSync throws if the process exits non-zero
		const e = err as { stdout?: string; stderr?: string; status?: number };
		stdout = e.stdout ?? '';
		stderr = e.stderr ?? '';
		exitCode = e.status ?? 1;
	}

	if (exitCode !== expectExit) {
		throw new Error(
			`curlew exited with ${exitCode} (expected ${expectExit})\nstdout:\n${stdout}\nstderr:\n${stderr}`
		);
	}

	// Parse result id from "Uploaded result res_..." line
	const resultIdMatch = stdout.match(/Uploaded result (res_[0-9a-f]+)/);
	const resultId = resultIdMatch?.[1];

	// Parse pass/fail counts from summary line "N passed, M failed"
	const passMatch = stdout.match(/(\d+)\s+passed/);
	const failMatch = stdout.match(/(\d+)\s+failed/);
	const pass = passMatch ? parseInt(passMatch[1], 10) : undefined;
	const fail = failMatch ? parseInt(failMatch[1], 10) : undefined;

	return { stdout, stderr, exitCode, resultId, pass, fail };
}
