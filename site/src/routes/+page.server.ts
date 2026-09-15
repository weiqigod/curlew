import { getAllExamples } from '$lib/content';
import { highlight } from '$lib/server/highlight';

const HERO = `name: Local echo
requests:
  - name: Echo a message
    request:
      method: POST
      url: "{{mud}}/echo"
      body:
        message: "hello"
    assertions:
      status: 200`;

const INSTALL = `# In an authenticated checkout; Go 1.24+ and Node.js 22+
./scripts/build-ui.sh
go build -o curlew ./cmd/curlew

# Start a project, then open the local browser UI
./curlew init demo-api
cd demo-api
../curlew ui`;

export const load = async () => {
	const examples = getAllExamples();
	const featuredSlugs = [
		'checkout-flow',
		'data-driven-provisioning',
		'graphql-fragments',
		'websocket-order-stream',
		'aws-sigv4-signing',
		'ci-pipeline'
	];
	const featured = featuredSlugs
		.map((s) => examples.find((e) => e.slug === s))
		.filter((e): e is NonNullable<typeof e> => Boolean(e));

	const [heroHtml, installHtml] = await Promise.all([
		highlight(HERO, 'yaml'),
		highlight(INSTALL, 'bash')
	]);

	return {
		examples,
		featured: featured.length ? featured : examples.slice(0, 6),
		heroHtml,
		installHtml
	};
};
