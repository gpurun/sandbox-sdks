package ai.gpurun.sandbox.boxapi;

import ai.gpurun.sandbox.boxapi.BoxApiModels.*;
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

import java.io.IOException;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;

public final class BoxApiClient {
    public static final String DEFAULT_BASE_URL = "http://127.0.0.1:8080";
    private static final MediaType JSON = MediaType.get("application/json; charset=utf-8");

    private final String baseUrl;
    private final String token;
    private final OkHttpClient httpClient;
    private final boolean ownsHttpClient;
    private final Headers headers;
    private final Gson gson = new Gson();

    private BoxApiClient(Builder builder) {
        this.baseUrl = firstNonEmpty(builder.baseUrl,
                envFirst("GPURUN_BOXAPI_URL"), DEFAULT_BASE_URL)
                .replaceAll("/+$", "");
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
                    .readTimeout(timeout, TimeUnit.SECONDS)
                    .writeTimeout(timeout, TimeUnit.SECONDS)
                    .callTimeout(timeout, TimeUnit.SECONDS)
                    .build();
            this.ownsHttpClient = true;
        }
    }

    public static Builder builder() { return new Builder(); }

    public BoxApiClient() { this(builder()); }

    public BoxApiClient(String baseUrl, String token, int timeoutSeconds, OkHttpClient httpClient, Map<String, String> headers) {
        this(builder().baseUrl(baseUrl).token(token).timeoutSeconds(timeoutSeconds)
                .httpClient(httpClient).headers(headers));
    }

    public String baseUrl() { return baseUrl; }

    public void close() {
        if (!ownsHttpClient) return;
        httpClient.dispatcher().executorService().shutdown();
        httpClient.connectionPool().evictAll();
        Cache cache = httpClient.cache();
        if (cache != null) {
            try { cache.close(); } catch (IOException ignored) { }
        }
    }

    public Health health() { return request("GET", "/health", null, null, Health.class); }

    public Sandbox createSandbox(CreateSandboxRequest req) {
        return request("POST", "/sandboxes", req, null, Sandbox.class);
    }

    public List<SandboxInfo> listSandboxes() {
        SandboxInfo[] items = request("GET", "/v1/sandboxes", null, null, SandboxInfo[].class);
        return items == null ? new ArrayList<>() : Arrays.asList(items);
    }

    public SandboxListPage listSandboxesV2(ListOptions opts) {
        ListOptions options = opts == null ? new ListOptions() : opts;
        Map<String, List<String>> query = new LinkedHashMap<>();
        List<String> states = options.getState();
        if (states != null && !states.isEmpty()) query.put("state", new ArrayList<>(states));
        query.put("nextToken", Collections.singletonList(String.valueOf(options.getNextToken())));
        int limit = options.getLimit() == null ? 100 : options.getLimit();
        query.put("limit", Collections.singletonList(String.valueOf(limit)));
        RawResponse raw = exchange("GET", "/sandboxes", null, query);
        SandboxListPage page = new SandboxListPage();
        if (raw.body != null && !raw.body.trim().isEmpty()) {
            try {
                JsonElement element = JsonParser.parseString(raw.body);
                if (element.isJsonArray()) {
                    SandboxListItem[] list = gson.fromJson(element, SandboxListItem[].class);
                    if (list != null) page.items = new ArrayList<>(Arrays.asList(list));
                } else if (element.isJsonObject()) {
                    JsonElement items = element.getAsJsonObject().get("items");
                    if (items != null && items.isJsonArray()) {
                        SandboxListItem[] list = gson.fromJson(items, SandboxListItem[].class);
                        if (list != null) page.items = new ArrayList<>(Arrays.asList(list));
                    }
                    JsonElement next = element.getAsJsonObject().get("nextToken");
                    if (next == null || next.isJsonNull()) next = element.getAsJsonObject().get("next_token");
                    if (next != null && next.isJsonPrimitive()) page.nextToken = next.getAsString();
                }
            } catch (JsonParseException e) {
                throw new BoxApiException(0, "invalid_response", "Invalid Box API JSON response", e);
            }
        }
        String next = raw.headers.get("X-Next-Token");
        if (next != null && !next.isEmpty()) page.nextToken = next;
        return page;
    }

    public SandboxInfo getSandbox(String id) {
        return request("GET", "/sandboxes/" + segment(id), null, null, SandboxInfo.class);
    }

    public void killSandbox(String id) { request("DELETE", "/sandboxes/" + segment(id), null, null, null); }

    public Snapshot pauseSandbox(String id) {
        return request("POST", "/v1/sandboxes/" + segment(id) + "/pause", null, null, Snapshot.class);
    }

    public SandboxInfo resumeSandbox(String id) {
        return request("POST", "/v1/sandboxes/" + segment(id) + "/resume", null, null, SandboxInfo.class);
    }

    public Sandbox connectSandbox(String id) {
        return request("POST", "/sandboxes/" + segment(id) + "/connect", null, null, Sandbox.class);
    }

    public void setSandboxTimeout(String id) { request("POST", "/sandboxes/" + segment(id) + "/timeout", null, null, null); }

    public SandboxInfo poweroffSandbox(String id) {
        return request("POST", "/v1/sandboxes/" + segment(id) + "/poweroff", null, null, SandboxInfo.class);
    }

    public SandboxInfo poweronSandbox(String id) {
        return request("POST", "/v1/sandboxes/" + segment(id) + "/poweron", null, null, SandboxInfo.class);
    }

    public SandboxInfo rebootSandbox(String id) {
        return request("POST", "/v1/sandboxes/" + segment(id) + "/reboot", null, null, SandboxInfo.class);
    }

    public BalloonConfig getBalloon(String id) {
        return request("GET", "/v1/sandboxes/" + segment(id) + "/balloon", null, null, BalloonConfig.class);
    }

    public BalloonConfig updateBalloon(String id, int amountMiB) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("amountMiB", amountMiB);
        return request("PATCH", "/v1/sandboxes/" + segment(id) + "/balloon", payload, null, BalloonConfig.class);
    }

    public BalloonStats getBalloonStats(String id) {
        return request("GET", "/v1/sandboxes/" + segment(id) + "/balloon/statistics", null, null, BalloonStats.class);
    }

    public BalloonConfig updateBalloonStats(String id, int intervalS) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("statsPollingIntervalS", intervalS);
        return request("PATCH", "/v1/sandboxes/" + segment(id) + "/balloon/statistics", payload, null, BalloonConfig.class);
    }

    public BalloonHinting getBalloonHinting(String id) {
        return request("GET", "/v1/sandboxes/" + segment(id) + "/balloon/hinting", null, null, BalloonHinting.class);
    }

    public BalloonHinting startBalloonHinting(String id, Boolean acknowledgeOnStop) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("acknowledgeOnStop", acknowledgeOnStop == null || acknowledgeOnStop);
        return request("POST", "/v1/sandboxes/" + segment(id) + "/balloon/hinting/start", payload, null, BalloonHinting.class);
    }

    public BalloonHinting startBalloonHinting(String id) { return startBalloonHinting(id, null); }

    public BalloonHinting stopBalloonHinting(String id) {
        return request("POST", "/v1/sandboxes/" + segment(id) + "/balloon/hinting/stop", null, null, BalloonHinting.class);
    }

    public List<CompatibleTemplate> listTemplates() {
        CompatibleTemplate[] items = request("GET", "/templates", null, null, CompatibleTemplate[].class);
        return items == null ? new ArrayList<>() : Arrays.asList(items);
    }

    public TemplateWithBuilds getTemplate(String id) {
        return request("GET", "/templates/" + segment(id), null, null, TemplateWithBuilds.class);
    }

    public void deleteTemplate(String id) { request("DELETE", "/templates/" + segment(id), null, null, null); }

    public TemplateBuildInfo getTemplateBuildStatus(String templateId, String buildId) {
        return request("GET", "/templates/" + segment(templateId) + "/builds/" + segment(buildId) + "/status",
                null, null, TemplateBuildInfo.class);
    }

    public void startTemplateBuildV2(String templateId, String buildId, Object req) {
        request("POST", "/v2/templates/" + segment(templateId) + "/builds/" + segment(buildId),
                req, null, null, 202);
    }

    public TemplateV3 createTemplateV3(Object req) {
        return request("POST", "/v3/templates", req, null, TemplateV3.class, 202);
    }

    public ConvertResult convertTemplate(Object req) {
        return request("POST", "/v1/templates/convert", req, null, ConvertResult.class, 201);
    }

    public ImageInfo createImage(CreateImageRequest req) {
        return request("POST", "/v1/images", req, null, ImageInfo.class, 201);
    }

    public ImageList listImages() { return request("GET", "/v1/images", null, null, ImageList.class); }

    public ImageInfo getImage(String id) {
        return request("GET", "/v1/images/" + segment(id), null, null, ImageInfo.class);
    }

    public void deleteImage(String id) { request("DELETE", "/v1/images/" + segment(id), null, null, null); }

    public RuntimeList listRuntimes() { return request("GET", "/v1/runtimes", null, null, RuntimeList.class); }

    public RuntimeSummary getRuntime(String type) {
        return request("GET", "/v1/runtimes/" + segment(type), null, null, RuntimeSummary.class);
    }

    public RuntimeCapabilities getRuntimeCapabilities(String type) {
        return request("GET", "/v1/runtimes/" + segment(type) + "/capabilities", null, null, RuntimeCapabilities.class);
    }

    public StrategyList listStrategies() { return request("GET", "/v1/scheduler/strategies", null, null, StrategyList.class); }

    public Strategy setStrategy(Object req) {
        return request("POST", "/v1/scheduler/strategies", req, null, Strategy.class);
    }

    public Strategy getStrategy(String id) {
        return request("GET", "/v1/scheduler/strategies/" + segment(id), null, null, Strategy.class);
    }

    public DeleteStrategyResult deleteStrategy(String id) {
        return request("DELETE", "/v1/scheduler/strategies/" + segment(id), null, null, DeleteStrategyResult.class);
    }

    public Strategy getDefaultStrategy() {
        return request("GET", "/v1/scheduler/strategies/default", null, null, Strategy.class);
    }

    public DefaultStrategyResult setDefaultStrategy(String id) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("id", id);
        return request("PUT", "/v1/scheduler/strategies/default", payload, null, DefaultStrategyResult.class);
    }

    public QuotaList getQuotas() { return request("GET", "/v1/scheduler/quotas", null, null, QuotaList.class); }

    public Quota updateQuota(Object req) {
        return request("PUT", "/v1/scheduler/quotas", req, null, Quota.class);
    }

    public Decision decide(DecideRequest req) {
        return request("POST", "/v1/scheduler/decide", req, null, Decision.class);
    }

    private <T> T request(String method, String path, Object payload, Map<String, List<String>> query, Class<T> type, int... allowed) {
        RawResponse raw = exchange(method, path, payload, query, allowed);
        return parse(raw.body, type);
    }

    private RawResponse exchange(String method, String path, Object payload, Map<String, List<String>> query, int... allowed) {
        String target = baseUrl + path;
        HttpUrl parsed = HttpUrl.parse(target);
        if (parsed == null) throw new BoxApiException(0, "invalid_url", "Invalid Box API URL: " + target);
        HttpUrl.Builder urlBuilder = parsed.newBuilder();
        if (query != null) {
            for (Map.Entry<String, List<String>> entry : query.entrySet()) {
                for (String value : entry.getValue()) urlBuilder.addQueryParameter(entry.getKey(), value);
            }
        }
        Request.Builder requestBuilder = new Request.Builder().url(urlBuilder.build()).headers(headers);
        RequestBody body = payload == null ? null : RequestBody.create(gson.toJson(payload), JSON);
        requestBuilder.method(method, body);
        try (Response response = httpClient.newCall(requestBuilder.build()).execute()) {
            byte[] data = response.body() == null ? new byte[0] : response.body().bytes();
            int status = response.code();
            if (!success(status) && !allowed(status, allowed)) throw error(status, data);
            return new RawResponse(new String(data, StandardCharsets.UTF_8), response.headers());
        } catch (IOException e) {
            throw new BoxApiException(0, "network_error", "Box API request failed: " + method + " " + path, e);
        }
    }

    private <T> T parse(String body, Class<T> type) {
        if (type == null || body == null || body.trim().isEmpty()) return null;
        try {
            JsonElement element = JsonParser.parseString(body);
            return gson.fromJson(element, type);
        } catch (JsonParseException e) {
            throw new BoxApiException(0, "invalid_response", "Invalid Box API JSON response", e);
        }
    }

    private static BoxApiException error(int status, byte[] data) {
        String text = new String(data, StandardCharsets.UTF_8).trim();
        String code = defaultCode(status);
        String message = text.isEmpty() ? "HTTP " + status : text;
        if (!text.isEmpty()) {
            try {
                JsonElement element = JsonParser.parseString(text);
                if (element.isJsonObject()) {
                    JsonObject object = element.getAsJsonObject();
                    if (object.has("code") && object.get("code").isJsonPrimitive()) code = object.get("code").getAsString();
                    if (object.has("message") && object.get("message").isJsonPrimitive()) message = object.get("message").getAsString();
                }
            } catch (JsonParseException ignored) { }
        }
        return new BoxApiException(status, code, message);
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
        if (value == null) throw new BoxApiException(0, "invalid_argument", "Path segment is required");
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

    public static final class Builder {
        private String baseUrl;
        private String token;
        private String apiKey;
        private int timeoutSeconds = 30;
        private OkHttpClient httpClient;
        private final Map<String, String> headers = new LinkedHashMap<>();

        private Builder() { }

        public Builder baseUrl(String value) { baseUrl = value; return this; }
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
        public BoxApiClient build() { return new BoxApiClient(this); }
    }

    private static final class RawResponse {
        final String body;
        final Headers headers;
        RawResponse(String body, Headers headers) { this.body = body; this.headers = headers; }
    }
}
