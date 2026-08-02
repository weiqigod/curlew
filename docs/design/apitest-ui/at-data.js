/* apitest UI — mock data: a payments API repo (paymux-api) */
window.AT = (function () {
  const ENVS = [
    { id: 'dev', base: 'https://api.dev.paymux.io' },
    { id: 'staging', base: 'https://api.staging.paymux.io' },
    { id: 'prod', base: 'https://api.paymux.io' },
  ];

  const R = (id, file, line, method, path, name) => ({ id, file, line, method, path, name });
  const REQ_LIST = [
    R('cust-create', 'collections/customers.yaml', 3, 'POST', '/v1/customers', 'Create customer'),
    R('cust-attach', 'collections/customers.yaml', 19, 'POST', '/v1/customers/{{customer.id}}/payment_methods', 'Attach payment method'),
    R('chg-list', 'collections/charges.yaml', 3, 'GET', '/v1/charges?limit=50', 'List charges'),
    R('chg-create', 'collections/charges.yaml', 14, 'POST', '/v1/charges', 'Create charge'),
    R('chg-get', 'collections/charges.yaml', 39, 'GET', '/v1/charges/{{charge.id}}', 'Get charge'),
    R('chg-capture', 'collections/charges.yaml', 52, 'POST', '/v1/charges/{{charge.id}}/capture', 'Capture charge'),
    R('chg-declined', 'collections/charges.yaml', 68, 'POST', '/v1/charges', 'Create charge — declined card'),
    R('ref-create', 'collections/refunds.yaml', 3, 'POST', '/v1/refunds', 'Create refund'),
    R('ref-get', 'collections/refunds.yaml', 24, 'GET', '/v1/refunds/{{refund.id}}', 'Get refund'),
    R('ref-partial', 'collections/refunds.yaml', 35, 'POST', '/v1/refunds', 'Partial refund over amount'),
    R('disp-list', 'collections/disputes.yaml', 3, 'GET', '/v1/disputes', 'List disputes'),
    R('disp-get', 'collections/disputes.yaml', 12, 'GET', '/v1/disputes/{{dispute.id}}', 'Get dispute'),
    R('disp-evidence', 'collections/disputes.yaml', 25, 'POST', '/v1/disputes/{{dispute.id}}/evidence', 'Submit evidence'),
    R('disp-close', 'collections/disputes.yaml', 47, 'POST', '/v1/disputes/{{dispute.id}}/close', 'Close dispute'),
  ];
  const REQUESTS = {};
  REQ_LIST.forEach((r) => (REQUESTS[r.id] = r));

  const FILES = [
    { path: 'collections/customers.yaml', requests: ['cust-create', 'cust-attach'] },
    { path: 'collections/charges.yaml', requests: ['chg-list', 'chg-create', 'chg-get', 'chg-capture', 'chg-declined'] },
    { path: 'collections/refunds.yaml', requests: ['ref-create', 'ref-get', 'ref-partial'] },
    { path: 'collections/disputes.yaml', requests: ['disp-list', 'disp-get', 'disp-evidence', 'disp-close'] },
    {
      path: 'collections/webhooks.yaml',
      requests: [],
      invalid: {
        diag: "webhooks.yaml: line 12: unknown operator 'eq' (did you mean 'equals'?)",
        line: 12,
        snippet: [
          [8, '  assert:'],
          [9, '    - status: { equals: 200 }'],
          [10, '    - headers.content-type:'],
          [11, '        contains: application/json'],
          [12, "    - body.$.pending_webhooks: { eq: 0 }"],
          [13, '    - body.$.livemode: { equals: false }'],
        ],
      },
    },
  ];

  const WAVES = [
    ['cust-create', 'chg-list', 'disp-list'],
    ['cust-attach', 'chg-declined', 'disp-get'],
    ['chg-create', 'disp-evidence'],
    ['chg-get', 'chg-capture', 'ref-partial'],
    ['ref-create', 'ref-get', 'disp-close'],
  ];

  const DEPS = {
    'cust-attach': 'cust-create',
    'chg-declined': 'cust-create',
    'chg-create': 'cust-attach',
    'chg-get': 'chg-create',
    'chg-capture': 'chg-create',
    'ref-partial': 'chg-create',
    'ref-create': 'chg-capture',
    'ref-get': 'ref-create',
    'disp-get': 'disp-list',
    'disp-evidence': 'disp-get',
    'disp-close': 'disp-evidence',
  };

  /* bias: request fails when bias < failRate (0..1). Defaults at 0.15:
     disp-evidence + ref-partial fail; disp-close skips. */
  const OUTCOMES = {
    'cust-create': { ms: 231, code: '201 Created', bias: 0.55 },
    'chg-list': { ms: 612, code: '200 OK', bias: 0.85 },
    'disp-list': { ms: 334, code: '200 OK', bias: 0.7 },
    'cust-attach': { ms: 189, code: '200 OK', bias: 0.4 },
    'chg-declined': { ms: 204, code: '402 Payment Required', bias: 0.9 },
    'disp-get': { ms: 492, code: '200 OK', bias: 0.65 },
    'chg-create': { ms: 245, code: '201 Created', bias: 0.35 },
    'disp-evidence': {
      ms: 884, code: '200 OK', bias: 0.05,
      fail: { code: '400 Bad Request', msg: 'body.$.evidence.status: expected "submitted", got "missing_fields"' },
    },
    'chg-get': { ms: 87, code: '200 OK', bias: 0.75 },
    'chg-capture': { ms: 356, code: '200 OK', bias: 0.5 },
    'ref-partial': {
      ms: 745, code: '201 Created', bias: 0.1,
      fail: { code: '201 Created', msg: 'body.$.amount: expected 100, got 95' },
    },
    'ref-create': { ms: 638, code: '201 Created', bias: 0.45 },
    'ref-get': { ms: 76, code: '200 OK', bias: 0.8 },
    'disp-close': { ms: 142, code: '200 OK', bias: 0.6 },
  };

  /* resolve outcomes for a given failure rate, with dependency skips */
  function resolve(failRate) {
    const out = {};
    WAVES.flat().forEach((id) => {
      const o = OUTCOMES[id];
      const dep = DEPS[id];
      if (dep && out[dep] && out[dep].result !== 'pass') {
        out[id] = { result: 'skip', ms: 0, code: '', msg: 'skipped — dependency ' + (out[dep].result === 'skip' ? 'skipped' : 'failed') + ': ' + REQUESTS[dep].name };
        return;
      }
      if (o.bias < failRate) {
        const f = o.fail || { code: '500 Internal Server Error', msg: 'status: expected 2xx, got 500' };
        out[id] = { result: 'fail', ms: o.ms, code: f.code, msg: f.msg };
      } else {
        out[id] = { result: 'pass', ms: o.ms, code: o.code };
      }
    });
    return out;
  }

  function completeRun(failRate) {
    const resolved = resolve(failRate == null ? 0.15 : failRate);
    const phases = {};
    Object.keys(resolved).forEach((id) => (phases[id] = resolved[id].result));
    let wall = 0;
    WAVES.forEach((w) => (wall += Math.max(...w.map((id) => (resolved[id].result === 'skip' ? 0 : OUTCOMES[id].ms)))));
    return { phases, resolved, running: false, wallMs: wall };
  }

  /* canonical mid-flight snapshot: waves 1–2 done, wave 3 running, 4–5 pending */
  function midflightRun() {
    const resolved = resolve(0.15);
    const phases = {};
    WAVES[0].concat(WAVES[1]).forEach((id) => (phases[id] = resolved[id].result));
    WAVES[2].forEach((id) => (phases[id] = 'running'));
    WAVES[3].concat(WAVES[4]).forEach((id) => (phases[id] = 'pending'));
    return { phases, resolved, running: true, wallMs: 1240 };
  }

  /* ---------- inspector payloads ---------- */
  const chargeBody = {
    id: 'ch_3NqR8eK2xT0a1Vb9',
    object: 'charge',
    amount: 4200,
    currency: 'usd',
    status: 'succeeded',
    captured: false,
    customer: 'cus_QfT81LbNa2',
    payment_method_details: {
      type: 'card',
      card: { brand: 'visa', last4: '4242', exp_month: 12, exp_year: 2027, network: 'visa', funding: 'credit' },
    },
    outcome: { network_status: 'approved_by_network', risk_level: 'normal', risk_score: 23, seller_message: 'Payment complete.' },
    billing_details: { name: 'Ada Lovelace', email: 'ada@example.com', address: { city: 'London', country: 'GB', postal_code: 'EC1A 1BB' } },
    metadata: { order_id: 'ord_18472', source: 'apitest' },
    livemode: false,
    created: 1718102400,
  };

  const listBody = {
    object: 'list',
    url: '/v1/charges',
    has_more: true,
    total_count: 412,
    data: Array.from({ length: 50 }, (_, i) => ({
      id: 'ch_3Nq' + (1000 + i * 7).toString(36).toUpperCase() + 'k' + i,
      object: 'charge',
      amount: 1200 + (i * 731) % 9000,
      currency: i % 7 === 0 ? 'eur' : 'usd',
      status: i % 11 === 3 ? 'failed' : 'succeeded',
      customer: 'cus_' + (8000 + i * 13).toString(36).toUpperCase(),
      created: 1718102400 - i * 8640,
    })),
  };

  const defaultHeaders = (ms) => [
    ['content-type', 'application/json; charset=utf-8'],
    ['x-request-id', 'req_Hh2k9PqYw4'],
    ['x-processing-ms', String(Math.max(1, Math.round(ms * 0.7)))],
    ['x-ratelimit-limit', '100'],
    ['x-ratelimit-remaining', '97'],
    ['x-ratelimit-reset', '1718102460'],
    ['cache-control', 'no-store'],
    ['strict-transport-security', 'max-age=63072000'],
  ];

  const timingFor = (ms) => {
    const dns = Math.round(ms * 0.06), conn = Math.round(ms * 0.13), tls = Math.round(ms * 0.21), dl = Math.round(ms * 0.06);
    return [
      ['DNS lookup', dns],
      ['TCP connect', conn],
      ['TLS handshake', tls],
      ['Waiting (TTFB)', ms - dns - conn - tls - dl],
      ['Content download', dl],
    ];
  };

  const INSPECT = {
    'chg-create': {
      body: chargeBody,
      timing: [['DNS lookup', 14], ['TCP connect', 31], ['TLS handshake', 52], ['Waiting (TTFB)', 134], ['Content download', 14]],
      assertions: [
        { expr: 'status equals 201', pass: true },
        { expr: 'body.$.status equals "succeeded"', pass: true },
        { expr: 'body.$.amount equals 4200', pass: true },
        { expr: 'body.$.payment_method_details.card.last4 equals "4242"', pass: true },
        { expr: 'headers.content-type contains "application/json"', pass: true },
        { expr: 'duration lessThan 500', pass: true },
      ],
      reqBody: { amount: 4200, currency: 'usd', customer: 'cus_QfT81LbNa2', capture: false, metadata: { order_id: 'ord_18472', source: 'apitest' } },
      yaml: [
        [14, '- name: Create charge'],
        [15, '  request:'],
        [16, '    method: POST'],
        [17, '    url: /v1/charges'],
        [18, '    headers:'],
        [19, '      idempotency-key: "{{uuid}}"'],
        [20, '    body:'],
        [21, '      amount: 4200'],
        [22, '      currency: usd'],
        [23, '      customer: "{{steps.create-customer.body.id}}"'],
        [24, '      capture: false'],
        [25, '  assert:'],
        [26, '    - status: { equals: 201 }'],
        [27, '    - body.$.status: { equals: succeeded }'],
        [28, '    - body.$.amount: { equals: 4200 }'],
      ],
    },
    'chg-list': {
      body: listBody,
      assertions: [
        { expr: 'status equals 200', pass: true },
        { expr: 'body.$.data length 50', pass: true },
        { expr: 'body.$.has_more equals true', pass: true },
      ],
      reqBody: null,
    },
    'ref-partial': {
      body: {
        id: 're_9HtQ3yMrXw2c',
        object: 'refund',
        amount: 95,
        charge: 'ch_3NqR8eK2xT0a1Vb9',
        currency: 'usd',
        status: 'pending',
        reason: 'requested_by_customer',
        balance_transaction: null,
        metadata: { order_id: 'ord_18472' },
        created: 1718103300,
      },
      assertions: [
        { expr: 'status equals 201', pass: true },
        { expr: 'body.$.amount equals 100', pass: false, expected: '100', actual: '95' },
        { expr: 'body.$.status equals "succeeded"', pass: false, expected: '"succeeded"', actual: '"pending"' },
      ],
      reqBody: { charge: 'ch_3NqR8eK2xT0a1Vb9', amount: 100, reason: 'requested_by_customer' },
      yaml: [
        [35, '- name: Partial refund over amount'],
        [36, '  request:'],
        [37, '    method: POST'],
        [38, '    url: /v1/refunds'],
        [39, '    body:'],
        [40, '      charge: "{{steps.create-charge.body.id}}"'],
        [41, '      amount: 100'],
        [42, '  assert:'],
        [43, '    - status: { equals: 201 }'],
        [44, '    - body.$.amount: { equals: 100 }'],
        [45, '    - body.$.status: { equals: succeeded }'],
      ],
    },
    'disp-evidence': {
      body: {
        error: {
          type: 'invalid_request_error',
          code: 'evidence_incomplete',
          message: 'Evidence is missing required fields: customer_signature, receipt.',
          doc_url: 'https://docs.paymux.io/errors/evidence_incomplete',
        },
        evidence: { status: 'missing_fields', missing: ['customer_signature', 'receipt'] },
      },
      assertions: [
        { expr: 'status equals 200', pass: false, expected: '200', actual: '400' },
        { expr: 'body.$.evidence.status equals "submitted"', pass: false, expected: '"submitted"', actual: '"missing_fields"' },
      ],
      reqBody: { product_description: 'Pro plan — annual', customer_email: 'ada@example.com' },
    },
  };

  function inspectFor(id, resolved) {
    const req = REQUESTS[id];
    const out = resolved[id] || { result: 'pass', ms: OUTCOMES[id].ms, code: OUTCOMES[id].code };
    const spec = INSPECT[id] || {};
    const objName = id.startsWith('cust') ? 'customer' : id.startsWith('ref') ? 'refund' : id.startsWith('disp') ? 'dispute' : 'charge';
    const body = spec.body || {
      id: objName.slice(0, 3) + '_8GkP2xLqWv4d',
      object: objName,
      status: out.result === 'fail' ? 'error' : 'succeeded',
      livemode: false,
      created: 1718102400,
    };
    return {
      req,
      out,
      body,
      headers: defaultHeaders(out.ms || OUTCOMES[id].ms),
      timing: spec.timing || timingFor(out.ms || OUTCOMES[id].ms),
      assertions: spec.assertions || [
        { expr: 'status equals ' + (OUTCOMES[id].code || '200').split(' ')[0], pass: out.result !== 'fail' },
        { expr: 'headers.content-type contains "application/json"', pass: true },
        { expr: 'duration lessThan 1000', pass: true },
      ],
      reqHeaders: [
        ['authorization', null /* redacted */],
        ['idempotency-key', 'idem_4f8a2c91'],
        ['content-type', 'application/json'],
        ['user-agent', 'apitest/0.9.2'],
        ['x-api-key', null /* redacted */],
      ],
      reqBody: 'reqBody' in spec ? spec.reqBody : null,
      yaml: spec.yaml || null,
    };
  }

  /* ---------- run history + comparison ---------- */
  const RUNS = [
    { id: 'run-1', time: '10:47', day: 'today', branch: 'feature/fix-totals', pass: 11, fail: 2, skip: 1, total: '3.4s', current: true },
    { id: 'run-2', time: '10:32', day: 'today', branch: 'main', pass: 14, fail: 0, skip: 0, total: '3.1s' },
    { id: 'run-3', time: '09:58', day: 'today', branch: 'main', pass: 13, fail: 1, skip: 0, total: '3.6s' },
    { id: 'run-4', time: '18:21', day: 'yesterday', branch: 'main', pass: 14, fail: 0, skip: 0, total: '3.0s' },
    { id: 'run-5', time: '16:02', day: 'yesterday', branch: 'feature/webhook-retry', pass: 12, fail: 2, skip: 0, total: '4.1s' },
  ];

  const COMPARE = {
    request: 'ref-partial',
    a: { run: 'run-2', status: 'pass', code: '201 Created', ms: 489 },
    b: { run: 'run-1', status: 'fail', code: '201 Created', ms: 745, failMsg: 'body.$.amount: expected 100, got 95' },
    changedKeys: 4,
    /* unified diff: t = ' ' context, '-' removed (run A), '+' added (run B) */
    diff: [
      { t: ' ', s: '{' },
      { t: '-', s: '  "id": "re_8GkP2xLqWv4d",' },
      { t: '+', s: '  "id": "re_9HtQ3yMrXw2c",' },
      { t: ' ', s: '  "object": "refund",' },
      { t: '-', s: '  "amount": 100,', k: '100' },
      { t: '+', s: '  "amount": 95,', k: '95' },
      { t: ' ', s: '  "charge": "ch_3NqR8eK2xT0a1Vb9",' },
      { t: ' ', s: '  "currency": "usd",' },
      { t: '-', s: '  "status": "succeeded",', k: '"succeeded"' },
      { t: '+', s: '  "status": "pending",', k: '"pending"' },
      { t: ' ', s: '  "reason": "requested_by_customer",' },
      { t: '-', s: '  "balance_transaction": "txn_1OqT3kJ9dQ",', k: '"txn_1OqT3kJ9dQ"' },
      { t: '+', s: '  "balance_transaction": null,', k: 'null' },
      { t: ' ', s: '  "metadata": {' },
      { t: ' ', s: '    "order_id": "ord_18472"' },
      { t: ' ', s: '  },' },
      { t: '-', s: '  "created": 1718101920' },
      { t: '+', s: '  "created": 1718103300' },
      { t: ' ', s: '}' },
    ],
  };

  return {
    ENVS, REQUESTS, REQ_LIST, FILES, WAVES, DEPS, OUTCOMES,
    resolve, completeRun, midflightRun, inspectFor, RUNS, COMPARE,
    project: 'paymux-api', branch: 'feature/fix-totals',
  };
})();
