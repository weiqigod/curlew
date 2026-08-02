import { getAllExamples } from '$lib/content';
import { highlight } from '$lib/server/highlight';

const HERO = `name: Checkout flow
requests:
  - name: Log in
    required: true
    request:
      method: POST
      url: "{{base_url}}/auth/login"
      body:
        email: "{{user_email}}"
        password: "{{user_password}}"   # redacted in every output
    assertions:
      status: 200
    extract:
      token: "$.access_token"            # feed it into the next request

  - name: Place order
    request:
      method: POST
      url: "{{base_url}}/orders"
      headers:
        Authorization: "Bearer {{token}}"
      body:
        idempotency_key: "{{$uuid}}"     # generated per run
        customer: "{{$faker.fullName}}"
        items:
          - { sku: "WIDGET-1", qty: 2 }
    assertions:
      status: 201
      body:
        "$.status": { equals: "confirmed" }
      cel:
        - "response.body.items.all(i, i.qty > 0)"`;

const INSTALL = `# Install — a single static binary, Go 1.24+
go install github.com/peterlindqvist/apitest/cmd/apitest@latest

# Run your first collection
apitest run collections/checkout.yaml --env staging`;

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
