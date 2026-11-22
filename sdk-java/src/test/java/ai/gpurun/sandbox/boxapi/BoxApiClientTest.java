package ai.gpurun.sandbox.boxapi;

import ai.gpurun.sandbox.boxapi.BoxApiModels.*;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class BoxApiClientTest {
    private MockWebServer server;
    private BoxApiClient client;

    @BeforeEach
    void start() throws IOException {
        server = new MockWebServer();
        server.start();
        client = BoxApiClient.builder().baseUrl(server.url("/").toString()).token("secret").timeoutSeconds(5).build();
    }

    @AfterEach
    void stop() throws Exception {
        client.close();
        server.shutdown();
    }

    private static MockResponse json(String body) {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody(body);
    }

    private static JsonObject body(RecordedRequest request) {
        return JsonParser.parseString(request.getBody().readUtf8()).getAsJsonObject();
    }

    @Test
    void healthUsesBearerToken() throws Exception {
        server.enqueue(json("{\"status\":\"ok\",\"service\":\"boxapi\"}"));
        Health health = client.health();
        assertEquals("ok", health.status);
        assertEquals("boxapi", health.service);
        RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("GET", request.getMethod());
        assertEquals("/health", request.getPath());
        assertEquals("Bearer secret", request.getHeader("Authorization"));
    }

    @Test
    void createSandboxUsesWireFieldNames() throws Exception {
        server.enqueue(json("{\"sandboxID\":\"sb-1\",\"clientID\":\"client-1\",\"envdVersion\":\"0.5.7\"}"));
        CreateSandboxRequest request = new CreateSandboxRequest();
        request.templateID = "tpl-1";
        request.imageId = "img-1";
        request.snapshotId = "snap-1";
        request.runtimeType = "firecracker";
        request.gpu = 2;
        request.timeout = 600;
        Map<String, String> metadata = new LinkedHashMap<>();
        metadata.put("team", "dev");
        request.metadata = metadata;
        Map<String, String> envVars = new LinkedHashMap<>();
        envVars.put("FOO", "bar");
        request.envVars = envVars;

        Sandbox sandbox = client.createSandbox(request);
        assertEquals("sb-1", sandbox.sandboxID);
        assertEquals("client-1", sandbox.clientID);

        RecordedRequest recorded = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("POST", recorded.getMethod());
        assertEquals("/sandboxes", recorded.getPath());
        JsonObject body = body(recorded);
        assertEquals("tpl-1", body.get("templateID").getAsString());
        assertEquals("img-1", body.get("image_id").getAsString());
        assertEquals("snap-1", body.get("snapshot_id").getAsString());
        assertEquals("firecracker", body.get("runtime_type").getAsString());
        assertEquals(2, body.get("gpu").getAsInt());
        assertEquals(600, body.get("timeout").getAsInt());
        assertEquals("dev", body.getAsJsonObject("metadata").get("team").getAsString());
        assertEquals("bar", body.getAsJsonObject("envVars").get("FOO").getAsString());
        assertFalse(body.has("imageId"));
    }

    @Test
    void listSandboxesV2ReadsHeaderTokenAndRepeatedStates() throws Exception {
        server.enqueue(json("[{\"sandboxID\":\"sb-1\",\"cpuCount\":2}]")
                .setHeader("X-Next-Token", "header-token"));
        ListOptions options = new ListOptions().setState(Arrays.asList("RUNNING", "PAUSED"))
                .setNextToken("7").setLimit(50);
        SandboxListPage page = client.listSandboxesV2(options);
        assertEquals("header-token", page.nextToken);
        assertEquals(1, page.items.size());
        assertEquals("sb-1", page.items.get(0).sandboxID);
        assertEquals(2, page.items.get(0).cpuCount);

        RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("/sandboxes?state=RUNNING&state=PAUSED&nextToken=7&limit=50", request.getPath());
    }

    @Test
    void listSandboxesV2AcceptsItemsEnvelope() throws Exception {
        server.enqueue(json("{\"items\":[{\"sandboxID\":\"sb-2\"}],\"nextToken\":\"body-token\"}"));
        SandboxListPage page = client.listSandboxesV2(new ListOptions());
        assertEquals("body-token", page.nextToken);
        assertEquals(1, page.items.size());
        assertEquals("sb-2", page.items.get(0).sandboxID);
    }

    @Test
    void decideUsesPascalCaseBody() throws Exception {
        server.enqueue(json("{\"provider\":\"aws\",\"strategy\":\"lowest\",\"score\":0.75}"));
        DecideRequest decide = new DecideRequest();
        decide.sandboxID = "sb-1";
        decide.templateID = "tpl-1";
        decide.imageID = "img-1";
        decide.labels = new LinkedHashMap<>();
        decide.labels.put("team", "dev");
        decide.annotations = new LinkedHashMap<>();
        decide.annotations.put("note", "test");
        decide.region = "us-east-1";
        decide.providerHint = "aws";
        DecisionRequirements requirements = new DecisionRequirements();
        requirements.minCpu = 4;
        requirements.minMemory = 8192;
        requirements.needGpu = true;
        requirements.region = "us-east-1";
        decide.requirements = requirements;

        Decision result = client.decide(decide);
        assertEquals("aws", result.provider);
        assertEquals(0.75, result.score);

        JsonObject body = body(server.takeRequest(2, TimeUnit.SECONDS));
        assertEquals("sb-1", body.get("SandboxID").getAsString());
        assertEquals("tpl-1", body.get("TemplateID").getAsString());
        assertEquals("img-1", body.get("ImageID").getAsString());
        assertEquals("dev", body.getAsJsonObject("Labels").get("team").getAsString());
        assertEquals("test", body.getAsJsonObject("Annotations").get("note").getAsString());
        assertEquals("us-east-1", body.get("Region").getAsString());
        assertEquals("aws", body.get("ProviderHint").getAsString());
        JsonObject nested = body.getAsJsonObject("Requirements");
        assertEquals(4, nested.get("MinCPU").getAsInt());
        assertEquals(8192, nested.get("MinMemory").getAsInt());
        assertTrue(nested.get("NeedGPU").getAsBoolean());
        assertEquals("us-east-1", nested.get("Region").getAsString());
        assertFalse(body.has("sandboxID"));
        assertFalse(body.has("requirements"));
    }

    @Test
    void failuresExposeStatusAndCode() throws Exception {
        server.enqueue(new MockResponse().setResponseCode(403)
                .setHeader("Content-Type", "application/json")
                .setBody("{\"code\":\"forbidden\",\"message\":\"access denied\"}"));
        BoxApiException error = assertThrows(BoxApiException.class, () -> client.getSandbox("sb-1"));
        assertEquals(403, error.getStatus());
        assertEquals("forbidden", error.getCode());
        assertEquals("access denied", error.getMessage());
    }

    @Test
    void killSandboxAcceptsEmptyResponse() throws Exception {
        server.enqueue(new MockResponse().setResponseCode(204));
        client.killSandbox("sb-1");
        RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
        assertEquals("DELETE", request.getMethod());
        assertEquals("/sandboxes/sb-1", request.getPath());
    }
}
