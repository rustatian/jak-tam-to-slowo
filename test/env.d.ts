import type { Env as WorkerEnv } from '../src/index';
import type { D1Migration } from 'cloudflare:test';

declare global {
	// web/api.ts dispatches the session-ended event on window. Workers types have no window.
	var window: EventTarget;
	namespace Cloudflare {
		interface Env extends WorkerEnv { TEST_MIGRATIONS: D1Migration[] }
		interface GlobalProps { mainModule: typeof import('../src/index') }
	}
}
