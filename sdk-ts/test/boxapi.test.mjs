import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import test from 'node:test';
import { BoxApiClient, BoxApiError, Gpurun } from '../dist/index.js';

async function withServer(handler, run) {
  const server = createServer(handler);
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  try {
    await run(`http://127.0.0.1:${address.port}`);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

function reply(res, status, value, headers = {}) {
  const body = value === undefined ? '' : JSON.stringify(value);
  res.writeHead(status, { 'Content-Type': 'application/json', ...headers });
  res.end(body);
}

async function body(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  const text = Buffer.concat(chunks).toString('utf8');
  return text === '' ? undefined : JSON.parse(text);
}

test('control plane sends bearer token and posts a bare create payload', async () => {
  let seen;
  await withServer(async (req, res) => {
    assert.equal(req.method, 'POST');
    assert.equal(req.url, '/sandboxes');
    assert.equal(req.headers.authorization, 'Bearer secret-key');
    seen = await body(req);
    reply(res, 200, {
      alias: null,
      clientID: 'client-1',
      domain: null,
      envdVersion: '1.0.0',
      sandboxID: 'sb-1',
      templateID: 'tpl-1',
      trafficAccessToken: null
    });
  }, async (baseUrl) => {
    const client = new BoxApiClient({ baseUrl, apiKey: 'secret-key' });
    const sandbox = await client.createSandbox({ templateID: 'tpl-1', metadata: { owner: 'qa' } });
    assert.equal(sandbox.sandboxID, 'sb-1');
    assert.deepEqual(seen, { templateID: 'tpl-1', metadata: { owner: 'qa' } });
  });
});

test('control plane validates create request source and rootfs', async () => {
  const client = new BoxApiClient({ baseUrl: 'http://127.0.0.1:1' });
  await assert.rejects(async () => client.createSandbox({}), (error) => error.code === 'invalid_argument');
  await assert.rejects(
    async () => client.createSandbox({ rootfs: { provider: 'oci', image: 'img' } }),
    (error) => error.code === 'invalid_argument'
  );
});

test('listSandboxesV2 sends filters and reads x-next-token', async () => {
  let query;
  await withServer(async (req, res) => {
    assert.equal(req.url.startsWith('/sandboxes?'), true);
    query = new URL(req.url, 'http://localhost').searchParams;
    reply(res, 200, [{ sandboxID: 'sb-2' }], { 'x-next-token': '25' });
  }, async (baseUrl) => {
    const client = new BoxApiClient({ baseUrl });
    const result = await client.listSandboxesV2({ state: ['running', 'paused'], nextToken: 5, limit: 5000 });
    assert.deepEqual(query.getAll('state'), ['running', 'paused']);
    assert.equal(query.get('nextToken'), '5');
    assert.equal(query.get('limit'), '1000');
    assert.equal(result.nextToken, '25');
    assert.equal(result.items[0].sandboxID, 'sb-2');
  });
});

test('listSandboxesV2 accepts items envelope fallback', async () => {
  await withServer(async (req, res) => {
    reply(res, 200, { items: [{ sandboxID: 'sb-3' }], nextToken: 'body-token' });
  }, async (baseUrl) => {
    const client = new BoxApiClient({ baseUrl });
    const result = await client.listSandboxesV2();
    assert.equal(result.nextToken, 'body-token');
    assert.equal(result.items[0].sandboxID, 'sb-3');
  });
});

test('listSandboxesV2 rejects invalid nextToken', async () => {
  const client = new BoxApiClient({ baseUrl: 'http://127.0.0.1:1' });
  await assert.rejects(() => client.listSandboxesV2({ nextToken: -1 }), (error) => error.code === 'invalid_argument');
});

test('control plane maps error responses to BoxApiError', async () => {
  await withServer(async (req, res) => {
    reply(res, 404, { code: 'sandbox_not_found', message: 'missing sandbox' });
  }, async (baseUrl) => {
    const client = new BoxApiClient({ baseUrl });
    await assert.rejects(() => client.getSandbox('missing'), (error) => {
      assert.ok(error instanceof BoxApiError);
      assert.equal(error.status, 404);
      assert.equal(error.code, 'sandbox_not_found');
      assert.equal(error.message, 'missing sandbox');
      return true;
    });
  });
});

test('decide uses PascalCase request body', async () => {
  let payload;
  await withServer(async (req, res) => {
    assert.equal(req.method, 'POST');
    assert.equal(req.url, '/v1/scheduler/decide');
    payload = await body(req);
    reply(res, 200, { provider: 'gpu-a', strategy: 'default', reason: 'ok', fallbacks: [], score: 1 });
  }, async (baseUrl) => {
    const client = new BoxApiClient({ baseUrl });
    const decision = await client.decide({
      sandboxID: 'sb-1',
      templateID: 'tpl-1',
      region: 'cn',
      requirements: { minCPU: 4, needGPU: true }
    });
    assert.deepEqual(payload, {
      SandboxID: 'sb-1',
      TemplateID: 'tpl-1',
      Region: 'cn',
      Requirements: { MinCPU: 4, NeedGPU: true }
    });
    assert.equal(decision.provider, 'gpu-a');
  });
});

test('facade exposes both clients and delegates create/exec', async () => {
  const gpurun = new Gpurun({ baseUrl: 'http://control', dataUrl: 'http://data', sandboxId: 'sb-1' });
  assert.equal(gpurun.sandboxId, 'sb-1');
  assert.equal(typeof gpurun.exec, 'function');
  assert.equal(typeof gpurun.createSandbox, 'function');
});


test('control plane uses server-compatible template and v1 sandbox routes', async () => {
  const seen = [];
  await withServer(async (req, res) => {
    seen.push(`${req.method} ${new URL(req.url, 'http://localhost').pathname}`);
    reply(res, 200, req.method === 'GET' && req.url === '/templates' ? [] : {});
  }, async (baseUrl) => {
    const client = new BoxApiClient({ baseUrl });
    const calls = [
      [() => client.listTemplates(), 'GET', '/templates'],
      [() => client.getTemplate('tpl'), 'GET', '/templates/tpl'],
      [() => client.deleteTemplate('tpl'), 'DELETE', '/templates/tpl'],
      [() => client.getTemplateBuildStatus('tpl', 'b'), 'GET', '/templates/tpl/builds/b/status'],
      [() => client.startTemplateBuildV2('tpl', 'b', {}), 'POST', '/v2/templates/tpl/builds/b'],
      [() => client.convertTemplate({}), 'POST', '/v1/templates/convert'],
      [() => client.poweroffSandbox('sb'), 'POST', '/v1/sandboxes/sb/poweroff'],
      [() => client.poweronSandbox('sb'), 'POST', '/v1/sandboxes/sb/poweron'],
      [() => client.rebootSandbox('sb'), 'POST', '/v1/sandboxes/sb/reboot'],
      [() => client.getBalloon('sb'), 'GET', '/v1/sandboxes/sb/balloon'],
      [() => client.updateBalloon('sb', { amountMiB: 512 }), 'PATCH', '/v1/sandboxes/sb/balloon'],
      [() => client.getBalloonStats('sb'), 'GET', '/v1/sandboxes/sb/balloon/statistics'],
      [() => client.updateBalloonStats('sb', { statsPollingIntervalS: 15 }), 'PATCH', '/v1/sandboxes/sb/balloon/statistics'],
      [() => client.getBalloonHinting('sb'), 'GET', '/v1/sandboxes/sb/balloon/hinting'],
      [() => client.startBalloonHinting('sb', {}), 'POST', '/v1/sandboxes/sb/balloon/hinting/start'],
      [() => client.stopBalloonHinting('sb'), 'POST', '/v1/sandboxes/sb/balloon/hinting/stop'],
      [() => client.connectSandbox('sb'), 'POST', '/sandboxes/sb/connect'],
      [() => client.setSandboxTimeout('sb'), 'POST', '/sandboxes/sb/timeout'],
    ];
    for (const [call, method, path] of calls) {
      seen.length = 0;
      await call();
      assert.deepEqual(seen, [`${method} ${path}`]);
    }
  });
});
