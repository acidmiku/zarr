import { defineConfig } from '@playwright/test';
export default defineConfig({
	testDir: './tests',
	timeout: 30_000,
	fullyParallel: true,
	reporter: 'list',
	retries: 0,
	use: {
		baseURL: 'http://127.0.0.1:4173',
		viewport: { width: 1600, height: 1000 },
		screenshot: 'only-on-failure',
		trace: 'retain-on-failure'
	},
	webServer: {
		command: 'npm run preview -- --host 127.0.0.1 --port 4173',
		url: 'http://127.0.0.1:4173',
		reuseExistingServer: !process.env.CI
	}
});
