package ai.gpurun.sandbox.boxd;

import ai.gpurun.sandbox.boxd.BoxdClient.ProcessStream;
import ai.gpurun.sandbox.boxd.BoxdModels.*;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import okio.Buffer;
import okio.ByteString;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class BoxdClientTest {
    private MockWebServer server;
    private BoxdClient client;

    @BeforeEach
    void start() throws IOException {
        server = new MockWebServer();
        server.start();
        client = BoxdClient.builder().baseUrl(server.url("/").toString())
                .sandboxId("sb-1").token("secret").timeoutSeconds(5).build();
    }

    @AfterEach
    void stop() throws Exception {
        client.close();
        server.shutdown();
    }

    private static MockResponse json(String body) {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody(body);
    }

    private static byte[] frame(int flags, String payload) {
        byte[] bytes = payload.getBytes(StandardCharsets.UTF_8);
        byte[] result = new byte[5 + bytes.length];
        result[0] = (byte) flags;
        result[1] = (byte) (bytes.length >>> 24);
        result[2] = (byte) (bytes.length >>> 16);
        result[3] = (byte) (bytes.length >>> 8);
        result[4] = (byte) bytes.length;
        System.arraycopy(bytes, 0, result, 5, bytes.length);
        return result;
    }

    @Test
    void execParsesNonZeroExitFrom422() throws Exception {
        server.enqueue(new MockResponse().setResponseCode(422)
                .setHeader("Content-Type", "application/json")
                .setBody("{\"exitCode\":7,\"stdout\":\"out\",\"stderr\":\"err\"}"));
        Map<String, String> env = new LinkedHashMap<>();
        env.put("FOO", "bar");
        ExecResult result = client.exec(Arrays.asList("sh", "-c", "exit 7"), env);
        assertEquals(7, result.exitCode);
        assertEquals("out", result.stdout);
        assertEquals("err", result.stderr);

        RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("/exec?sandboxID=sb-1", request.getPath());
        assertEquals("sb-1", request.getHeader("Novita-Sandbox-Id"));
        assertEquals("Bearer secret", request.getHeader("Authorization"));
        JsonObject body = JsonParser.parseString(request.getBody().readUtf8()).getAsJsonObject();
        assertEquals("exit 7", body.getAsJsonArray("cmd").get(2).getAsString());
        assertEquals("bar", body.getAsJsonObject("envVars").get("FOO").getAsString());
    }

    @Test
    void startProcessDecodesEnvelope() throws Exception {
        server.enqueue(json("{\"process\":{\"pid\":42,\"tag\":\"job\"}}").setResponseCode(201));
        StartProcessRequest request = new StartProcessRequest();
        request.cmd = Arrays.asList("sleep", "1");
        request.tag = "job";
        ProcessInfo info = client.startProcess(request);
        assertEquals(42, info.pid);
        assertEquals("job", info.tag);
        RecordedRequest recorded = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("/processes?sandboxID=sb-1", recorded.getPath());
        assertEquals("POST", recorded.getMethod());
    }

    @Test
    void connectListUsesConnectContentType() throws Exception {
        server.enqueue(json("{\"processes\":[{\"pid\":9,\"config\":{\"cmd\":\"sh\"}}]}"));
        List<ConnectProcessInfo> processes = client.listProcessesConnect();
        assertEquals(1, processes.size());
        assertEquals(9, processes.get(0).pid);
        assertEquals("sh", processes.get(0).config.cmd);
        RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("/process.Process/List?sandboxID=sb-1", request.getPath());
        assertEquals("application/connect+json", request.getHeader("Content-Type"));
        assertEquals("sb-1", request.getHeader("Novita-Sandbox-Id"));
    }

    @Test
    void streamConsumesFramesUntilEnd() throws Exception {
        Buffer body = new Buffer();
        body.write(frame(0, "{\"event\":{\"start\":{\"pid\":11}}}"));
        body.write(frame(0, "{\"event\":{\"data\":{\"stdout\":\"aGk=\",\"stderr\":\"\",\"pty\":\"\"}}}"));
        body.write(frame(0, "{\"event\":{\"keepalive\":{}}}"));
        body.write(frame(2, "{\"process\":{\"pid\":11},\"event\":{\"end\":{\"exitCode\":3,\"exited\":true,\"status\":\"exited\"}}}"));
        server.enqueue(new MockResponse().setHeader("Content-Type", "application/connect+json").setBody(body));
        List<ProcessEvent> events = new ArrayList<>();
        StartProcessConnectRequest request = new StartProcessConnectRequest();
        ProcessStreamConfig config = new ProcessStreamConfig();
        config.cmd = "sh";
        request.process = config;

        try (ProcessStream stream = client.startProcessStream(request, events::add)) {
            ProcessExit exit = stream.awaitEnd(3, TimeUnit.SECONDS);
            assertEquals(3, exit.exitCode);
            assertTrue(exit.exited);
            assertEquals("exited", exit.status);
        }
        assertEquals(4, events.size());
        assertNotNull(events.get(0).event.start);
        assertEquals(11, events.get(0).event.start.pid);
        assertEquals("aGk=", events.get(1).event.data.stdout);
        assertNotNull(events.get(2).event.keepalive);
        assertEquals(3, events.get(3).event.end.exitCode);
        RecordedRequest recorded = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("/process.Process/Start?sandboxID=sb-1", recorded.getPath());
        assertEquals("application/connect+json", recorded.getHeader("Content-Type"));
    }

    @Test
    void decodeFrameValidatesHeaderAndLength() {
        BoxdClient.Frame frame = BoxdClient.decodeFrame(frame(0, "{}"));
        assertEquals(0, frame.flags);
        assertEquals("{}", new String(frame.payload, StandardCharsets.UTF_8));
        assertThrows(BoxdException.class, () -> BoxdClient.decodeFrame(new byte[]{0, 0, 0}));
        byte[] truncated = frame(0, "{}");
        assertThrows(BoxdException.class, () -> BoxdClient.decodeFrame(Arrays.copyOf(truncated, 6)));
        byte[] oversized = {0, 0x7f, (byte) 0xff, (byte) 0xff, (byte) 0xff};
        assertThrows(BoxdException.class, () -> BoxdClient.decodeFrame(oversized));
    }

    @Test
    void healthWorksWithoutSandboxIdButExecStillRequiresIt() throws Exception {
        server.enqueue(json("{\"status\":\"ok\",\"service\":\"boxproxy\"}"));
        BoxdClient noSandbox = BoxdClient.builder().baseUrl(server.url("/").toString()).build();
        try {
            BoxHealth health = noSandbox.health();
            assertEquals("ok", health.status);
            RecordedRequest recorded = server.takeRequest(2, TimeUnit.SECONDS);
            assertEquals("/healthz", recorded.getPath());
            assertEquals(null, recorded.getHeader("Novita-Sandbox-Id"));

            assertThrows(BoxdException.class, () -> noSandbox.exec(Arrays.asList("true"), null));
            assertEquals(1, server.getRequestCount());
        } finally {
            noSandbox.close();
        }
    }

    @Test
    void connectProcessToleratesBinaryEventFrames() throws Exception {
        server.enqueue(new MockResponse().withWebSocketUpgrade(new WebSocketListener() {
            @Override public void onOpen(WebSocket webSocket, Response response) {
                webSocket.send(ByteString.of("{\"type\":\"start\",\"process\":{\"pid\":7}}".getBytes(StandardCharsets.UTF_8)));
                webSocket.send(ByteString.of("hello".getBytes(StandardCharsets.UTF_8)));
                webSocket.send(ByteString.of("{\"type\":\"end\",\"exit\":{\"exitCode\":3,\"exited\":true,\"status\":\"done\"}}".getBytes(StandardCharsets.UTF_8)));
            }
        }));
        StringBuilder output = new StringBuilder();
        List<ProcessWSEvent> events = new ArrayList<>();
        BoxdClient.ProcessSocket socket = client.connectProcess("p1",
                bytes -> output.append(new String(bytes, StandardCharsets.UTF_8)),
                events::add);
        try {
            ProcessExit exit = socket.awaitEnd(3, TimeUnit.SECONDS);
            assertEquals(3, exit.exitCode);
            assertEquals("done", exit.status);
            assertEquals("hello", output.toString());
            assertEquals(2, events.size());
            assertEquals("start", events.get(0).type);
            assertNotNull(events.get(0).process);
            assertEquals(7, events.get(0).process.get("pid").getAsInt());
            assertEquals("end", events.get(1).type);
            assertNotNull(events.get(1).exit);
            assertEquals(3, events.get(1).exit.exitCode);
        } finally {
            socket.close();
        }
    }

    @Test
    void sendInputBase64EncodesPlaintextPayload() throws Exception {
        server.enqueue(new MockResponse().setResponseCode(200));
        SendInputRequest request = new SendInputRequest();
        request.process = new ProcessSelector(42);
        request.input = new SendInputPayload();
        request.input.stdin = "hi";
        request.input.pty = "\u0003";
        client.sendInput(request);
        RecordedRequest recorded = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("/process.Process/SendInput?sandboxID=sb-1", recorded.getPath());
        JsonObject body = JsonParser.parseString(recorded.getBody().readUtf8()).getAsJsonObject();
        assertEquals(42, body.getAsJsonObject("process").get("pid").getAsInt());
        assertEquals("aGk=", body.getAsJsonObject("input").get("stdin").getAsString());
        assertEquals("Aw==", body.getAsJsonObject("input").get("pty").getAsString());
    }
}
