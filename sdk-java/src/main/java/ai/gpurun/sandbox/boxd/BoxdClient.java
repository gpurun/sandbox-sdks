package ai.gpurun.sandbox.boxd;

import ai.gpurun.sandbox.boxd.BoxdModels.*;
import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParseException;
import com.google.gson.JsonParser;
import okhttp3.Cache;
import okhttp3.Headers;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;
import okio.ByteString;

import java.io.IOException;
import java.io.InputStream;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

public final class BoxdClient {
    public static final String DEFAULT_BASE_URL = "http://127.0.0.1:8082";
    public static final String SANDBOX_ID_HEADER = "Novita-Sandbox-Id";
    public static final String CONNECT_CONTENT_TYPE = "application/connect+json";
    private static final MediaType JSON = MediaType.get("application/json; charset=utf-8");
    private static final MediaType CONNECT = MediaType.get(CONNECT_CONTENT_TYPE);
    private static final long MAX_FRAME_LENGTH = 64L * 1024 * 1024;
    private static final Gson STATIC_GSON = new Gson();

    private final String baseUrl;
    private final String sandboxId;
    private final String token;
    private final OkHttpClient httpClient;
    private final boolean ownsHttpClient;
    private final Headers headers;
    private final Gson gson = new Gson();

    private BoxdClient(Builder builder) {
        this.baseUrl = firstNonEmpty(builder.baseUrl,
                envFirst("GPURUN_BOXD_URL"),
                DEFAULT_BASE_URL).replaceAll("/+$", "");
        this.sandboxId = firstNonEmpty(builder.sandboxId,
                envFirst("GPURUN_SANDBOX_ID", "NOVITA_SANDBOX_ID"));
        this.token = firstNonEmpty(builder.token, builder.apiKey,
                envFirst("GPURUN_API_KEY", "GPURUN_ACCESS_TOKEN", "NOVITA_API_KEY", "NOVITA_ACCESS_TOKEN"));
        Headers.Builder headerBuilder = new Headers.Builder();
        headerBuilder.set("Accept", "application/json");
        for (Map.Entry<String, String> entry : builder.headers.entrySet()) {
            headerBuilder.set(entry.getKey(), entry.getValue());
        }
        if (token != null && !token.isEmpty()) headerBuilder.set("Authorization", "Bearer " + token);
        this.headers = headerBuilder.build();
        if (builder.httpClient != null) {
            this.httpClient = builder.httpClient;
            this.ownsHttpClient = false;
        } else {
            int timeout = builder.timeoutSeconds;
            this.httpClient = new OkHttpClient.Builder()
                    .connectTimeout(timeout, TimeUnit.SECONDS)
                    .readTimeout(Math.max(timeout, 30), TimeUnit.SECONDS)
                    .writeTimeout(timeout, TimeUnit.SECONDS)
                    .callTimeout(0, TimeUnit.SECONDS)
                    .build();
            this.ownsHttpClient = true;
        }
    }

    public static Builder builder() { return new Builder(); }

    public BoxdClient() { this(builder()); }

    public BoxdClient(String baseUrl, String sandboxId, int timeoutSeconds, OkHttpClient httpClient, Map<String, String> headers) {
        this(builder().baseUrl(baseUrl).sandboxId(sandboxId).timeoutSeconds(timeoutSeconds)
                .httpClient(httpClient).headers(headers));
    }

    public String baseUrl() { return baseUrl; }
    public String sandboxId() { return sandboxId; }

    public void close() {
        if (!ownsHttpClient) return;
        httpClient.dispatcher().executorService().shutdown();
        httpClient.connectionPool().evictAll();
        Cache cache = httpClient.cache();
        if (cache != null) {
            try { cache.close(); } catch (IOException ignored) { }
        }
    }

    public BoxHealth health() {
        RawResponse raw = exchange("GET", "/healthz", null, JSON, false);
        if (raw.body.trim().isEmpty()) return null;
        try {
            return gson.fromJson(raw.body, BoxHealth.class);
        } catch (JsonParseException e) {
            throw new BoxdException(0, "invalid_response", "Invalid boxd JSON response", e);
        }
    }

    public ExecResult exec(List<String> cmd, Map<String, String> envVars) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("cmd", cmd == null ? Collections.emptyList() : cmd);
        payload.put("envVars", envVars == null ? Collections.emptyMap() : envVars);
        RawResponse raw = exchange("POST", "/exec", payload, JSON, 422);
        ExecResult result = new ExecResult();
        result.exitCode = raw.status == 422 ? 1 : 0;
        JsonElement element = parseLenient(raw.body);
        if (element == null) {
            if (!raw.body.trim().isEmpty()) result.stdout = raw.body;
            return result;
        }
        if (!element.isJsonObject()) {
            result.stdout = element.isJsonPrimitive() ? element.getAsString() : raw.body;
            return result;
        }
        JsonObject object = element.getAsJsonObject();
        Integer exitCode = intValue(object, "exitCode", "exit_code", "code");
        if (exitCode != null) result.exitCode = exitCode;
        String stdout = stringValue(object, "stdout", "output");
        if (stdout != null) result.stdout = stdout;
        String stderr = stringValue(object, "stderr", "error");
        if (stderr != null) result.stderr = stderr;
        return result;
    }

    public List<ProcessInfo> listProcesses() {
        RawResponse raw = exchange("GET", "/processes", null, JSON);
        JsonElement element = parseLenient(raw.body);
        if (element != null && element.isJsonArray()) {
            ProcessInfo[] items = gson.fromJson(element, ProcessInfo[].class);
            return items == null ? new ArrayList<>() : Arrays.asList(items);
        }
        if (element != null && element.isJsonObject()) {
            JsonElement processes = element.getAsJsonObject().get("processes");
            if (processes != null && processes.isJsonArray()) {
                ProcessInfo[] items = gson.fromJson(processes, ProcessInfo[].class);
                return items == null ? new ArrayList<>() : Arrays.asList(items);
            }
        }
        throw new BoxdException(0, "invalid_response", "Invalid process list response");
    }

    public ProcessInfo startProcess(StartProcessRequest req) {
        RawResponse raw = exchange("POST", "/processes", req, JSON, 201);
        return decodeProcess(raw.body);
    }

    public ProcessInfo waitProcess(String id) {
        RawResponse raw = exchange("GET", "/processes/" + segment(id) + "/wait", null, JSON);
        return decodeProcess(raw.body);
    }

    public void resizeProcess(String id, int rows, int cols) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("rows", rows);
        payload.put("cols", cols);
        exchange("POST", "/processes/" + segment(id) + "/resize", payload, JSON);
    }

    public void signalProcess(String id, String signal) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("signal", signal);
        exchange("POST", "/processes/" + segment(id) + "/signal", payload, JSON);
    }

    public ProcessSocket connectProcess(String id, ProcessOutputHandler handler) {
        return connectProcess(id, handler, null);
    }

    public ProcessSocket connectProcess(String id, ProcessOutputHandler handler, ProcessSocketEventHandler onEvent) {
        HttpUrl target = target("/processes/" + segment(id) + "/connect");
        Request.Builder requestBuilder = new Request.Builder().url(target).headers(headers)
                .header(SANDBOX_ID_HEADER, requireSandboxId()).get();
        ProcessSocket socket = new ProcessSocket(handler, onEvent);
        WebSocket webSocket = httpClient.newWebSocket(requestBuilder.build(), socket.listener());
        socket.attach(webSocket);
        return socket;
    }

    public List<ConnectProcessInfo> listProcessesConnect() {
        RawResponse raw = exchange("POST", "/process.Process/List", new JsonObject(), CONNECT);
        JsonElement element = parseLenient(raw.body);
        if (element != null && element.isJsonObject()) {
            JsonElement processes = element.getAsJsonObject().get("processes");
            if (processes != null && processes.isJsonArray()) {
                ConnectProcessInfo[] items = gson.fromJson(processes, ConnectProcessInfo[].class);
                return items == null ? new ArrayList<>() : Arrays.asList(items);
            }
        }
        throw new BoxdException(0, "invalid_response", "Invalid Connect process list response");
    }

    public ProcessStream startProcessStream(StartProcessConnectRequest req, ProcessEventHandler onEvent) {
        return new ProcessStream(requestStream("POST", "/process.Process/Start", req), onEvent);
    }

    public ProcessStream connectProcessStream(ConnectRequest req, ProcessEventHandler onEvent) {
        return new ProcessStream(requestStream("POST", "/process.Process/Connect", req), onEvent);
    }

    public void updateProcess(UpdateProcessRequest req) {
        exchange("POST", "/process.Process/Update", req, CONNECT);
    }

    public void sendInput(SendInputRequest req) {
        if (req != null && req.input != null) {
            if (req.input.stdin != null && !req.input.stdin.isEmpty()) {
                req.input.stdin = Base64.getEncoder().encodeToString(req.input.stdin.getBytes(StandardCharsets.UTF_8));
            }
            if (req.input.pty != null && !req.input.pty.isEmpty()) {
                req.input.pty = Base64.getEncoder().encodeToString(req.input.pty.getBytes(StandardCharsets.UTF_8));
            }
        }
        exchange("POST", "/process.Process/SendInput", req, CONNECT);
    }

    public void sendSignal(SendSignalRequest req) {
        exchange("POST", "/process.Process/SendSignal", req, CONNECT);
    }

    private <T> T request(String method, String path, Object payload, Class<T> type, MediaType contentType, int... allowed) {
        RawResponse raw = exchange(method, path, payload, contentType, allowed);
        if (type == null || raw.body.trim().isEmpty()) return null;
        try {
            return gson.fromJson(raw.body, type);
        } catch (JsonParseException e) {
            throw new BoxdException(0, "invalid_response", "Invalid boxd JSON response", e);
        }
    }

    private RawResponse exchange(String method, String path, Object payload, MediaType contentType, int... allowed) {
        return exchange(method, path, payload, contentType, true, allowed);
    }

    private RawResponse exchange(String method, String path, Object payload, MediaType contentType, boolean requireSandbox, int... allowed) {
        RequestBody body = payload == null ? null
                : RequestBody.create(gson.toJson(payload).getBytes(StandardCharsets.UTF_8), contentType);
        Request.Builder builder = requestBuilder(path, contentType, requireSandbox).method(method, body);
        try (Response response = httpClient.newCall(builder.build()).execute()) {
            byte[] data = response.body() == null ? new byte[0] : response.body().bytes();
            int status = response.code();
            if (!success(status) && !allowed(status, allowed)) throw error(status, data);
            return new RawResponse(status, new String(data, StandardCharsets.UTF_8));
        } catch (IOException e) {
            throw new BoxdException(0, "network_error", "Boxd request failed: " + method + " " + path, e);
        }
    }

    private Response requestStream(String method, String path, Object payload) {
        RequestBody body = RequestBody.create(
                gson.toJson(payload == null ? new JsonObject() : payload).getBytes(StandardCharsets.UTF_8), CONNECT);
        Request.Builder builder = requestBuilder(path, CONNECT).method(method, body);
        try {
            Response response = httpClient.newCall(builder.build()).execute();
            if (!success(response.code())) {
                String detail = response.body() == null ? "" : response.body().string();
                int status = response.code();
                response.close();
                throw error(status, detail.getBytes(StandardCharsets.UTF_8));
            }
            return response;
        } catch (IOException e) {
            throw new BoxdException(0, "network_error", "Boxd stream failed: " + path, e);
        }
    }

    private Request.Builder requestBuilder(String path, MediaType contentType) {
        return requestBuilder(path, contentType, true);
    }

    private Request.Builder requestBuilder(String path, MediaType contentType, boolean requireSandbox) {
        HttpUrl url = target(path, requireSandbox);
        Request.Builder builder = new Request.Builder().url(url).headers(headers)
                .header("Accept", contentType.toString());
        if (requireSandbox) {
            builder.header(SANDBOX_ID_HEADER, requireSandboxId());
        } else if (sandboxId != null && !sandboxId.isEmpty()) {
            builder.header(SANDBOX_ID_HEADER, sandboxId);
        }
        return builder;
    }

    private HttpUrl target(String path) {
        return target(path, true);
    }

    private HttpUrl target(String path, boolean requireSandbox) {
        String target = baseUrl + path;
        HttpUrl parsed = HttpUrl.parse(target);
        if (parsed == null) throw new BoxdException(0, "invalid_url", "Invalid boxd URL: " + target);
        if (requireSandbox) {
            return parsed.newBuilder().addQueryParameter("sandboxID", requireSandboxId()).build();
        }
        if (sandboxId != null && !sandboxId.isEmpty()) {
            return parsed.newBuilder().addQueryParameter("sandboxID", sandboxId).build();
        }
        return parsed;
    }

    private String requireSandboxId() {
        if (sandboxId == null || sandboxId.isEmpty()) {
            throw new BoxdException(400, "invalid_argument", "sandboxID is required");
        }
        return sandboxId;
    }

    private ProcessInfo decodeProcess(String body) {
        JsonElement element = parseLenient(body);
        if (element != null && element.isJsonObject()) {
            JsonElement process = element.getAsJsonObject().get("process");
            if (process != null && process.isJsonObject()) return gson.fromJson(process, ProcessInfo.class);
        }
        if (element != null) return gson.fromJson(element, ProcessInfo.class);
        throw new BoxdException(0, "invalid_response", "Invalid process response");
    }

    public static Frame decodeFrame(byte[] data) {
        if (data == null || data.length < 5) throw new BoxdException(502, "invalid_response", "Connect frame is truncated");
        int flags = data[0] & 0xff;
        long length = ((long) (data[1] & 0xff) << 24)
                | ((long) (data[2] & 0xff) << 16)
                | ((long) (data[3] & 0xff) << 8)
                | (long) (data[4] & 0xff);
        if (length > MAX_FRAME_LENGTH) throw new BoxdException(502, "invalid_response", "Connect frame too large");
        if (data.length < 5 + length) throw new BoxdException(502, "invalid_response", "Connect frame is truncated");
        return new Frame(flags, Arrays.copyOfRange(data, 5, 5 + (int) length));
    }

    public static ProcessEvent parseEvent(byte[] payload) {
        if (payload == null || payload.length == 0) return null;
        String text = new String(payload, StandardCharsets.UTF_8);
        try {
            JsonElement element = JsonParser.parseString(text);
            if (!element.isJsonObject()) throw new JsonParseException("Connect event is not an object");
            return STATIC_GSON.fromJson(element, ProcessEvent.class);
        } catch (JsonParseException e) {
            throw new BoxdException(502, "invalid_response", "Invalid Connect event frame", e);
        }
    }

    public static ProcessExit parseExit(JsonObject object) {
        ProcessExit exit = new ProcessExit();
        if (object == null || object.size() == 0) {
            exit.exited = true;
            return exit;
        }
        Integer code = intValue(object, "exitCode", "exit_code", "code");
        if (code != null) exit.exitCode = code;
        JsonElement exited = object.get("exited");
        if (exited != null && exited.isJsonPrimitive()) {
            String text = exited.getAsString();
            exit.exited = "true".equalsIgnoreCase(text) || "1".equals(text);
        } else {
            exit.exited = true;
        }
        String status = stringValue(object, "status");
        if (status != null) exit.status = status;
        String error = stringValue(object, "error", "message");
        if (error != null) exit.error = error;
        return exit;
    }

    private static JsonElement parseLenient(String body) {
        if (body == null || body.trim().isEmpty()) return null;
        try {
            return JsonParser.parseString(body);
        } catch (JsonParseException e) {
            return null;
        }
    }

    private static Integer intValue(JsonObject object, String... keys) {
        for (String key : keys) {
            JsonElement value = object.get(key);
            if (value == null || !value.isJsonPrimitive()) continue;
            String text = value.getAsString().trim();
            try {
                return Integer.valueOf(text);
            } catch (NumberFormatException ignored) { }
        }
        return null;
    }

    private static String stringValue(JsonObject object, String... keys) {
        for (String key : keys) {
            JsonElement value = object.get(key);
            if (value != null && value.isJsonPrimitive()) return value.getAsString();
        }
        return null;
    }

    private static BoxdException error(int status, byte[] data) {
        String text = new String(data, StandardCharsets.UTF_8).trim();
        String message = text.isEmpty() ? "HTTP " + status : text;
        JsonElement element = parseLenient(text);
        if (element != null && element.isJsonObject()) {
            JsonObject object = element.getAsJsonObject();
            JsonElement field = object.get("message");
            if (field == null) field = object.get("error");
            if (field != null && field.isJsonPrimitive()) message = field.getAsString();
            else if (field != null && field.isJsonObject()) {
                JsonElement nested = field.getAsJsonObject().get("message");
                if (nested != null && nested.isJsonPrimitive()) message = nested.getAsString();
            } else {
                JsonElement detail = object.get("detail");
                if (detail != null && detail.isJsonPrimitive()) message = detail.getAsString();
            }
        }
        return new BoxdException(status, defaultCode(status), message);
    }

    private static String defaultCode(int status) {
        switch (status) {
            case 400: return "bad_request";
            case 401: return "unauthorized";
            case 403: return "forbidden";
            case 404: return "not_found";
            case 409: return "conflict";
            case 422: return "unprocessable_entity";
            default: return status >= 500 ? "internal_error" : "http_error";
        }
    }

    private static boolean success(int status) { return status >= 200 && status < 300; }

    private static boolean allowed(int status, int... values) {
        for (int value : values) if (status == value) return true;
        return false;
    }

    private static String segment(String value) {
        if (value == null) throw new BoxdException(400, "invalid_argument", "Process ID is required");
        return URLEncoder.encode(value, StandardCharsets.UTF_8).replace("+", "%20");
    }

    private static String firstNonEmpty(String... values) {
        for (String value : values) if (value != null && !value.trim().isEmpty()) return value.trim();
        return null;
    }

    private static String envFirst(String... keys) {
        for (String key : keys) {
            String value = System.getenv(key);
            if (value != null && !value.trim().isEmpty()) return value.trim();
        }
        return null;
    }

    private static byte[] readFully(InputStream input, int length) throws IOException {
        byte[] data = new byte[length];
        int offset = 0;
        while (offset < length) {
            int read = input.read(data, offset, length - offset);
            if (read < 0) return null;
            offset += read;
        }
        return data;
    }

    public static final class Frame {
        public final int flags;
        public final byte[] payload;
        public Frame(int flags, byte[] payload) { this.flags = flags; this.payload = payload; }
    }

    public final class ProcessStream implements AutoCloseable {
        private final Response response;
        private final ProcessEventHandler handler;
        private final CountDownLatch done = new CountDownLatch(1);
        private final AtomicReference<ProcessExit> exit = new AtomicReference<>();
        private final AtomicReference<BoxdException> failure = new AtomicReference<>();
        private final AtomicBoolean finished = new AtomicBoolean(false);
        private final AtomicBoolean closed = new AtomicBoolean(false);

        private ProcessStream(Response response, ProcessEventHandler handler) {
            this.response = response;
            this.handler = handler;
            Thread reader = new Thread(this::run, "boxd-process-stream");
            reader.setDaemon(true);
            reader.start();
        }

        public ProcessExit awaitEnd() { return awaitEnd(30, TimeUnit.SECONDS); }

        public ProcessExit awaitEnd(long timeout, TimeUnit unit) {
            try {
                if (!done.await(timeout, unit)) throw new BoxdException(504, "timeout", "Timed out waiting for process end");
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new BoxdException(0, "interrupted", "Interrupted waiting for process end", e);
            }
            BoxdException error = failure.get();
            if (error != null) throw error;
            return exit.get();
        }

        @Override public void close() {
            closed.set(true);
            response.close();
            finish(null, new BoxdException(499, "closed", "process stream closed by client"));
        }

        public boolean isClosed() { return closed.get(); }

        private void run() {
            if (response.body() == null) {
                finish(null, new BoxdException(502, "invalid_response", "Connect stream has no body"));
                return;
            }
            InputStream input = response.body().byteStream();
            try {
                while (!closed.get()) {
                    byte[] header = readFully(input, 5);
                    if (header == null) throw new BoxdException(502, "invalid_response", "Unexpected EOF in Connect frame");
                    long length = ((long) (header[1] & 0xff) << 24)
                            | ((long) (header[2] & 0xff) << 16)
                            | ((long) (header[3] & 0xff) << 8)
                            | (long) (header[4] & 0xff);
                    if (length > MAX_FRAME_LENGTH) throw new BoxdException(502, "invalid_response", "Connect frame too large");
                    byte[] payload = readFully(input, (int) length);
                    if (payload == null) throw new BoxdException(502, "invalid_response", "Connect frame is truncated");
                    byte[] full = new byte[5 + (int) length];
                    System.arraycopy(header, 0, full, 0, 5);
                    System.arraycopy(payload, 0, full, 5, payload.length);
                    Frame frame = decodeFrame(full);
                    ProcessEvent event = parseEvent(frame.payload);
                    if (handler != null && event != null) handler.onEvent(event);
                    ProcessExit end = event == null || event.event == null ? null : event.event.end;
                    if ((frame.flags & 0x02) != 0 || end != null) {
                        finish(end == null ? parseExit(null) : end, null);
                        return;
                    }
                }
            } catch (BoxdException e) {
                finish(null, e);
            } catch (IOException e) {
                finish(null, new BoxdException(502, "invalid_response", "Failed reading Connect stream", e));
            } catch (RuntimeException e) {
                finish(null, new BoxdException(502, "invalid_response", "Failed parsing Connect stream", e));
            }
        }

        private void finish(ProcessExit result, BoxdException error) {
            if (!finished.compareAndSet(false, true)) return;
            exit.set(result);
            failure.set(error);
            done.countDown();
        }
    }

    public final class ProcessSocket implements AutoCloseable {
        private final ProcessOutputHandler handler;
        private final ProcessSocketEventHandler onEvent;
        private final CountDownLatch done = new CountDownLatch(1);
        private final AtomicReference<ProcessExit> exit = new AtomicReference<>();
        private final AtomicReference<BoxdException> failure = new AtomicReference<>();
        private final AtomicBoolean finished = new AtomicBoolean(false);
        private final AtomicBoolean closed = new AtomicBoolean(false);
        private volatile WebSocket webSocket;

        private ProcessSocket(ProcessOutputHandler handler, ProcessSocketEventHandler onEvent) {
            this.handler = handler;
            this.onEvent = onEvent;
        }

        private void attach(WebSocket value) { webSocket = value; }

        private WebSocketListener listener() {
            return new WebSocketListener() {
                @Override public void onMessage(WebSocket socket, ByteString bytes) {
                    byte[] data = bytes.toByteArray();
                    // boxproxy 会把 TEXT 事件帧改写成 BINARY，先探测 JSON 事件。
                    if (isEventPayload(data)) {
                        handleEventText(new String(data, StandardCharsets.UTF_8));
                        return;
                    }
                    if (handler != null) handler.onOutput(data);
                }
                @Override public void onMessage(WebSocket socket, String text) {
                    handleEventText(text);
                }
                @Override public void onFailure(WebSocket socket, Throwable t, okhttp3.Response response) {
                    finish(null, new BoxdException(response == null ? 0 : response.code(),
                            "websocket_error", "WebSocket failure: " + t.getMessage(), t));
                }
                @Override public void onClosed(WebSocket socket, int code, String reason) {
                    finish(null, new BoxdException(499, "closed", "process socket closed"));
                }
            };
        }

        private void handleEventText(String text) {
            JsonObject object = parseLenientObject(text);
            if (object == null) {
                finish(null, new BoxdException(502, "invalid_response", "Invalid WebSocket JSON frame"));
                return;
            }
            ProcessWSEvent event = new ProcessWSEvent();
            event.type = stringValue(object, "type");
            JsonElement process = object.get("process");
            if (process != null && process.isJsonObject()) {
                event.process = process.getAsJsonObject();
            }
            if (!"end".equals(event.type)) {
                if (onEvent != null) onEvent.onEvent(event);
                return;
            }
            JsonElement exitElement = object.get("exit");
            ProcessExit result = parseExit(exitElement != null && exitElement.isJsonObject()
                    ? exitElement.getAsJsonObject() : null);
            event.exit = result;
            if (onEvent != null) onEvent.onEvent(event);
            finish(result, null);
        }

        public void send(ByteString data) {
            WebSocket socket = webSocket;
            if (socket == null || closed.get()) throw new BoxdException(499, "closed", "process socket is closed");
            socket.send(data);
        }

        public void send(byte[] data) { send(ByteString.of(data)); }

        @Override public void close() {
            closed.set(true);
            WebSocket socket = webSocket;
            if (socket != null) socket.close(1000, null);
            finish(null, new BoxdException(499, "closed", "process socket closed by client"));
        }

        public boolean isClosed() { return closed.get(); }

        public ProcessExit awaitEnd() { return awaitEnd(30, TimeUnit.SECONDS); }

        public ProcessExit awaitEnd(long timeout, TimeUnit unit) {
            try {
                if (!done.await(timeout, unit)) throw new BoxdException(504, "timeout", "Timed out waiting for process end");
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new BoxdException(0, "interrupted", "Interrupted waiting for process end", e);
            }
            BoxdException error = failure.get();
            if (error != null) throw error;
            return exit.get();
        }

        private void finish(ProcessExit result, BoxdException error) {
            if (!finished.compareAndSet(false, true)) return;
            exit.set(result);
            failure.set(error);
            done.countDown();
        }
    }

    private static JsonObject parseLenientObject(String text) {
        JsonElement element = parseLenient(text);
        return element != null && element.isJsonObject() ? element.getAsJsonObject() : null;
    }

    private static boolean isEventPayload(byte[] data) {
        if (data == null || data.length == 0) return false;
        JsonElement element;
        try {
            element = JsonParser.parseString(new String(data, StandardCharsets.UTF_8));
        } catch (JsonParseException e) {
            return false;
        }
        if (element == null || !element.isJsonObject()) return false;
        JsonElement type = element.getAsJsonObject().get("type");
        if (type == null || !type.isJsonPrimitive()) return false;
        switch (type.getAsString()) {
            case "start":
            case "data":
            case "end":
            case "keepalive":
                return true;
            default:
                return false;
        }
    }

    public interface ProcessOutputHandler {
        void onOutput(byte[] output);
    }

    public interface ProcessEventHandler {
        void onEvent(ProcessEvent event);
    }

    /** Receives flat WebSocket process events (start/data/keepalive/end). */
    public interface ProcessSocketEventHandler {
        void onEvent(ProcessWSEvent event);
    }

    public static final class Builder {
        private String baseUrl;
        private String sandboxId;
        private String token;
        private String apiKey;
        private int timeoutSeconds = 30;
        private OkHttpClient httpClient;
        private final Map<String, String> headers = new LinkedHashMap<>();

        private Builder() { }

        public Builder baseUrl(String value) { baseUrl = value; return this; }
        public Builder sandboxId(String value) { sandboxId = value; return this; }
        public Builder token(String value) { token = value; return this; }
        public Builder apiKey(String value) { apiKey = value; return this; }
        public Builder accessToken(String value) { return token(value); }
        public Builder timeoutSeconds(int value) {
            if (value < 0) throw new IllegalArgumentException("timeoutSeconds must be non-negative");
            timeoutSeconds = value;
            return this;
        }
        public Builder timeout(int value, TimeUnit unit) { return timeoutSeconds((int) unit.toSeconds(value)); }
        public Builder httpClient(OkHttpClient value) { httpClient = value; return this; }
        public Builder headers(Map<String, String> value) {
            headers.clear();
            if (value != null) headers.putAll(value);
            return this;
        }
        public Builder header(String name, String value) { headers.put(name, value); return this; }
        public BoxdClient build() { return new BoxdClient(this); }
    }

    private static final class RawResponse {
        final int status;
        final String body;
        RawResponse(int status, String body) { this.status = status; this.body = body; }
    }
}
