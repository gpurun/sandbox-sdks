import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { createServer } from 'node:http';
import test from 'node:test';
import { BoxdClient, BoxdError, decodeDataField } from '../dist/index.js';

async function withServer(handler, run) {
  const server = createServer(handler);
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  try {
    await run(`http://127.0.0.1:${address.port}`);
  } finally {
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
}

async function readBody(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  const text = Buffer.concat(chunks).toString('utf8');
  return text === '' ? undefined : JSON.parse(text);
}

function connectFrame(flags, value) {
  const payload = Buffer.from(JSON.stringify(value));
  const header = Buffer.alloc(5);
  header[0] = flags;
  header.writeUInt32BE(payload.length, 1);
  return Buffer.concat([header, payload]);
}

function wsFrame(payload, opcode) {
  const data = Buffer.isBuffer(payload) ? payload : Buffer.from(payload);
  let header;
  if (data.length < 126) {
    header = Buffer.from([0x80 | opcode, data.length]);
  } else if (data.length <= 0xffff) {
    header = Buffer.alloc(4);
    header[0] = 0x80 | opcode;
    header[1] = 126;
    header.writeUInt16BE(data.length, 2);
  } else {
    header = Buffer.alloc(10);
    header[0] = 0x80 | opcode;
    header[1] = 127;
    header.writeBigUInt64BE(BigInt(data.length), 2);
  }
  return Buffer.concat([header, data]);
}

test('data plane sends sandboxID header and query and parses exec 200/422', async () => {
  const seen = [];
  await withServer(async (req, res) => {
    assert.equal(req.url.startsWith('/exec?sandboxID=sb-1'), true);
    assert.equal(req.headers['novita-sandbox-id'], 'sb-1');
    const payload = await readBody(req);
    seen.push(payload);
    if (payload.cmd[0] === 'fail') {
      res.writeHead(422, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ exitCode: 7, stdout: 'partial', stderr: 'failed' }));
      return;
    }
    if (payload.cmd[0] === 'error') {
      res.writeHead(400, { 'Content-Type': 'text/plain' });
      res.end('bad command');
      return;
    }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ exitCode: 0, stdout: 'ok', stderr: '' }));
  }, async (baseUrl) => {
    const client = new BoxdClient({ baseUrl, sandboxId: 'sb-1' });
    assert.deepEqual(await client.exec(['echo', 'ok'], { A: 'B' }), { exitCode: 0, stdout: 'ok', stderr: '' });
    assert.deepEqual(await client.exec(['fail']), { exitCode: 7, stdout: 'partial', stderr: 'failed' });
    await assert.rejects(() => client.exec(['error']), (error) => {
      assert.ok(error instanceof BoxdError);
      assert.equal(error.status, 400);
      assert.equal(error.message, 'bad command');
      return true;
    });
    assert.deepEqual(seen[0], { cmd: ['echo', 'ok'], envVars: { A: 'B' } });
  });
});

test('Connect stream parses split five-byte frames and resolves end', async () => {
  let contentType;
  let requestBody;
  let query;
  let sandboxHeader;
  const events = [];
  await withServer(async (req, res) => {
    assert.equal(req.url.startsWith('/process.Process/Start?sandboxID=sb-2'), true);
    query = new URL(req.url, 'http://localhost').searchParams;
    sandboxHeader = req.headers['novita-sandbox-id'];
    contentType = req.headers['content-type'];
    requestBody = await readBody(req);

    const start = connectFrame(0, { event: { start: { pid: 42 } } });
    const data = connectFrame(0, { event: { data: { stdout: 'b3V0' } } });
    const end = connectFrame(2, { event: { end: { exitCode: 3, exited: true, status: 'failed', error: 'oops' } } });
    res.writeHead(200, { 'Content-Type': 'application/connect+json' });
    res.write(start.subarray(0, 3));
    setTimeout(() => res.end(Buffer.concat([start.subarray(3), data, end])), 5);
  }, async (baseUrl) => {
    const client = new BoxdClient({ baseUrl, sandboxId: 'sb-2' });
    const stream = await client.startProcessStream(
      { process: { cmd: 'sh', args: ['-c', 'echo out'] }, stdin: true },
      (event) => events.push(event)
    );
    const end = await stream.waitEnd();
    assert.equal(end.exitCode, 3);
    assert.equal(end.exited, true);
    assert.equal(end.status, 'failed');
    assert.equal(end.error, 'oops');
    assert.equal(events.length, 3);
    assert.equal(events[0].event.start.pid, 42);
    assert.equal(events[1].event.data.stdout, 'b3V0');
    assert.equal(events[2].event.end.exitCode, 3);
    assert.equal(contentType, 'application/connect+json');
    assert.equal(query.get('sandboxID'), 'sb-2');
    assert.equal(sandboxHeader, 'sb-2');
    assert.equal(requestBody.process.cmd, 'sh');
  });
});

test('WebSocket process protocol carries sandbox header, binary input and end frame', async () => {
  const server = createServer();
  const sockets = new Set();
  let upgradeUrl;
  let sandboxHeader;
  let clientFrame;

  server.on('upgrade', (req, socket) => {
    sockets.add(socket);
    upgradeUrl = req.url;
    sandboxHeader = req.headers['novita-sandbox-id'];
    const key = req.headers['sec-websocket-key'];
    const accept = createHash('sha1')
      .update(key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11')
      .digest('base64');
    socket.write(
      `HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`
    );
    socket.write(wsFrame(JSON.stringify({ type: 'start', process: { pid: 7 } }), 0x01));
    socket.write(wsFrame(Buffer.from('ping'), 0x02));

    let replied = false;
    socket.on('data', (chunk) => {
      if (!clientFrame) clientFrame = Buffer.from(chunk);
      if (replied) return;
      replied = true;
      socket.write(
        wsFrame(JSON.stringify({ type: 'end', exit: { exitCode: 0, exited: true, status: 'ok' } }), 0x01)
      );
      setTimeout(() => socket.end(), 20);
    });
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  try {
    const client = new BoxdClient({ baseUrl: `http://127.0.0.1:${address.port}`, sandboxId: 'sb-ws' });
    const started = [];
    const received = [];
    const ends = [];
    const socket = await client.connectProcess('pid-1', {
      onStart: (process) => started.push(process.pid),
      onData: (data) => received.push(Buffer.from(data).toString('utf8')),
      onEnd: (end) => ends.push(end.status)
    });
    socket.send('abc');
    const result = await socket.waitEnd();

    assert.equal(upgradeUrl, '/processes/pid-1/connect?sandboxID=sb-ws');
    assert.equal(sandboxHeader, 'sb-ws');
    assert.deepEqual(started, [7]);
    assert.deepEqual(received, ['ping']);
    assert.deepEqual(ends, ['ok']);
    assert.equal(result.exitCode, 0);
    assert.equal(clientFrame[0] & 0x0f, 0x02);
    assert.equal(clientFrame[1] & 0x80, 0x80);
    const mask = clientFrame.subarray(2, 6);
    const payload = Buffer.from(clientFrame.subarray(6));
    for (let index = 0; index < payload.length; index += 1) payload[index] ^= mask[index % 4];
    assert.equal(payload.toString('utf8'), 'abc');
    socket.close();
  } finally {
    for (const socket of sockets) socket.destroy();
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
});

test('sendInput base64 encodes plaintext stdin and pty', async () => {
  let requestBody;
  let contentType;
  await withServer(async (req, res) => {
    assert.equal(req.url.startsWith('/process.Process/SendInput?sandboxID=sb-1'), true);
    contentType = req.headers['content-type'];
    requestBody = await readBody(req);
    res.writeHead(204);
    res.end();
  }, async (baseUrl) => {
    const client = new BoxdClient({ baseUrl, sandboxId: 'sb-1' });
    await client.sendInput({ process: { pid: 7 }, input: { stdin: 'hi', pty: 'x' } });
    assert.equal(contentType, 'application/connect+json');
    assert.deepEqual(requestBody, { process: { pid: 7 }, input: { stdin: 'aGk=', pty: 'eA==' } });
  });
});

test('health works without sandboxID', async () => {
  let path;
  let sandboxHeader;
  await withServer(async (req, res) => {
    path = req.url;
    sandboxHeader = req.headers['novita-sandbox-id'];
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ status: 'ok', service: 'boxproxy' }));
  }, async (baseUrl) => {
    const client = new BoxdClient({ baseUrl });
    const health = await client.health();
    assert.equal(health.status, 'ok');
    assert.equal(health.service, 'boxproxy');
    assert.equal(path, '/healthz');
    assert.equal(sandboxHeader, undefined);
    await assert.rejects(() => client.exec(['true']), (error) => {
      assert.equal(error.message, 'sandboxID is required');
      return true;
    });
  });
});

test('connectProcess tolerates binary event frames rewritten by boxproxy', async () => {
  const server = createServer();
  const sockets = new Set();

  server.on('upgrade', (req, socket) => {
    sockets.add(socket);
    const key = req.headers['sec-websocket-key'];
    const accept = createHash('sha1')
      .update(key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11')
      .digest('base64');
    socket.write(
      `HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`
    );
    socket.write(wsFrame(JSON.stringify({ type: 'start', process: { pid: 9 } }), 0x02));
    socket.write(wsFrame(Buffer.from('out'), 0x02));
    socket.write(wsFrame(JSON.stringify({ type: 'end', exit: { exitCode: 5, exited: true, status: 'done' } }), 0x02));
    setTimeout(() => socket.end(), 20);
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  try {
    const client = new BoxdClient({ baseUrl: `http://127.0.0.1:${address.port}`, sandboxId: 'sb-ws' });
    const received = [];
    const ends = [];
    const socket = await client.connectProcess('pid-1', {
      onData: (data) => received.push(Buffer.from(data).toString('utf8')),
      onEnd: (end) => ends.push(end.status)
    });
    const result = await socket.waitEnd();
    assert.deepEqual(received, ['out']);
    assert.deepEqual(ends, ['done']);
    assert.equal(result.exitCode, 5);
    socket.close();
  } finally {
    for (const socket of sockets) socket.destroy();
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
});

test('decodeDataField decodes base64 connect data fields', () => {
  assert.deepEqual(Buffer.from(decodeDataField('aGVsbG8=')).toString('utf8'), 'hello');
  assert.equal(decodeDataField(undefined), undefined);
  assert.equal(decodeDataField(''), undefined);
  assert.throws(() => decodeDataField('!!!'), (error) => error instanceof BoxdError && error.status === 502);
});
