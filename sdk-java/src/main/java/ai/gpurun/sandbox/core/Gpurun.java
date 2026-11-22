package ai.gpurun.sandbox.core;

import ai.gpurun.sandbox.boxapi.BoxApiClient;
import ai.gpurun.sandbox.boxd.BoxdClient;

import java.util.concurrent.CompletableFuture;

/** Root facade for the Gpurun Sandbox Java SDK. */
public class Gpurun implements AutoCloseable {
    private final SandboxOpts options;
    private final ApiClient client;
    private final Template templates;
    private final Volume volumes;
    private final Secret secrets;
    private final Job jobs;
    public final BoxApiClient box;
    public final BoxdClient data;
    private volatile boolean closed;

    protected Gpurun(SandboxOpts options) throws SandboxException {
        this.options = options.copy();
        this.client = new ApiClient(this.options);
        this.templates = new Template(client);
        this.volumes = new Volume(client);
        this.secrets = new Secret(client);
        this.jobs = new Job(client);
        this.box = BoxApiClient.builder()
                .baseUrl(this.options.getBoxApiBaseUrl())
                .token(this.options.getBoxToken())
                .apiKey(this.options.getBoxApiKey())
                .timeoutSeconds(this.options.getRequestTimeoutSeconds())
                .headers(this.options.getHeaders())
                .build();
        this.data = BoxdClient.builder()
                .baseUrl(this.options.getBoxDataUrl())
                .sandboxId(this.options.getSandboxId())
                .token(this.options.getBoxToken())
                .apiKey(this.options.getBoxApiKey())
                .timeoutSeconds(this.options.getRequestTimeoutSeconds())
                .headers(this.options.getHeaders())
                .build();
    }

    public static Gpurun create() throws SandboxException { return new Gpurun(new SandboxOptions()); }

    public static Gpurun create(SandboxOptions options) throws SandboxException { return new Gpurun(options == null ? new SandboxOptions() : options); }

    public static Gpurun create(SandboxOpts options) throws SandboxException { return new Gpurun(options == null ? new SandboxOpts() : options); }

    public Template templates() { return templates; }
    public Job job() { return jobs; }
    public Job jobs() { return jobs; }
    public Volume volumes(){return volumes;} public Secret secrets(){return secrets;}
    public Template template(){return templates;} public Volume volume(){return volumes;} public Secret secret(){return secrets;}
    public BoxApiClient box() { return box; }
    public BoxdClient data() { return data; }
    public Sandbox createSandbox(String template) throws SandboxException { return Sandbox.create(template, options); }
    public Sandbox connectSandbox(String id) throws SandboxException { return Sandbox.connect(id, options); }
    public SandboxInfo[] listSandboxes() throws SandboxException { return client.listSandboxes(); }
    public SandboxListResult listSandboxes(SandboxListOptions options) throws SandboxException { return client.listSandboxesPage(options); }
    public SandboxInfo[] listAllSandboxes(SandboxListOptions options) throws SandboxException { return client.listAllSandboxes(options); }
    public SandboxInfo[] listAllSandboxes() throws SandboxException { return listAllSandboxes(null); }
    public CompletableFuture<SandboxListResult> listSandboxesAsync(SandboxListOptions options) { return AsyncSupport.supplyAsync(() -> { try{return listSandboxes(options);}catch(Exception e){throw new java.util.concurrent.CompletionException(e);} }); }
    public CompletableFuture<SandboxInfo[]> listAllSandboxesAsync(SandboxListOptions options) { return AsyncSupport.supplyAsync(() -> { try{return listAllSandboxes(options);}catch(Exception e){throw new java.util.concurrent.CompletionException(e);} }); }
    public CompletableFuture<Sandbox> createSandboxAsync(String template) { return AsyncSupport.supplyAsync(() -> { try{return createSandbox(template);}catch(Exception e){throw new java.util.concurrent.CompletionException(e);} }); }
    public CompletableFuture<Sandbox> connectSandboxAsync(String id) { return AsyncSupport.supplyAsync(() -> { try{return connectSandbox(id);}catch(Exception e){throw new java.util.concurrent.CompletionException(e);} }); }
    public CompletableFuture<SandboxInfo[]> listSandboxesAsync() { return AsyncSupport.supplyAsync(() -> { try{return listSandboxes();}catch(Exception e){throw new java.util.concurrent.CompletionException(e);} }); }

    @Override public void close() {
        if (closed) return;
        closed = true;
        box.close();
        data.close();
        client.close();
    }
}
