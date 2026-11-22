package ai.gpurun.sandbox.core;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class GpurunFacadeTest {
    @Test
    void facadeExposesBoxClients() throws Exception {
        SandboxOptions options = new SandboxOptions().apiKey("local-key")
                .boxApiBaseUrl("http://127.0.0.1:18080")
                .boxDataUrl("http://127.0.0.1:18082")
                .sandboxId("sb-1");
        try (Gpurun sdk = Gpurun.create(options)) {
            assertNotNull(sdk.box);
            assertNotNull(sdk.data);
            assertEquals("http://127.0.0.1:18080", sdk.box.baseUrl());
            assertEquals("http://127.0.0.1:18082", sdk.data.baseUrl());
            assertEquals("sb-1", sdk.data.sandboxId());
            assertTrue(sdk.box == sdk.box());
            assertTrue(sdk.data == sdk.data());
        }
    }

    @Test
    void legacyNovitaFacadeStillResolves() throws Exception {
        try (Novita client = Novita.create(new SandboxOptions().apiKey("local-key"))) {
            assertTrue(client instanceof Gpurun);
            assertNotNull(client.box);
            assertNotNull(client.data);
        }
    }
}
