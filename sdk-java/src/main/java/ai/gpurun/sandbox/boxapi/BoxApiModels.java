package ai.gpurun.sandbox.boxapi;

import com.google.gson.annotations.SerializedName;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

public final class BoxApiModels {
    private BoxApiModels() {
    }

    public static class Health {
        public String status;
        public String service;
        public long startedAtUnixNano;
    }

    public static class RootfsSource {
        public String provider;
        public String image;
        public String pullMode;
    }

    public static class RootfsInfo extends RootfsSource {
        public String path;
    }

    public static class CreateSandboxRequest {
        public String templateID;
        @SerializedName("image_id")
        public String imageId;
        @SerializedName("snapshot_id")
        public String snapshotId;
        public RootfsSource rootfs;
        @SerializedName("runtime_type")
        public String runtimeType;
        public Integer gpu;
        public Map<String, String> metadata;
        public Map<String, String> envVars;
        public Integer timeout;

        public CreateSandboxRequest templateID(String value) { templateID = value; return this; }
        public CreateSandboxRequest imageId(String value) { imageId = value; return this; }
        public CreateSandboxRequest snapshotId(String value) { snapshotId = value; return this; }
        public CreateSandboxRequest rootfs(RootfsSource value) { rootfs = value; return this; }
        public CreateSandboxRequest runtimeType(String value) { runtimeType = value; return this; }
        public CreateSandboxRequest gpu(Integer value) { gpu = value; return this; }
        public CreateSandboxRequest metadata(Map<String, String> value) { metadata = value; return this; }
        public CreateSandboxRequest envVars(Map<String, String> value) { envVars = value; return this; }
        public CreateSandboxRequest timeout(Integer value) { timeout = value; return this; }
    }

    public static class Sandbox {
        public String alias;
        public String clientID;
        public String domain;
        public String envdAccessToken;
        public String envdVersion;
        public String sandboxID;
        public String templateID;
        public String trafficAccessToken;
    }

    public static class SandboxInfo {
        public String sandboxID;
        public String templateID;
        public String imageID;
        public String snapshotID;
        public String state;
        public String runtimeType;
        public Map<String, String> metadata;
        public RootfsInfo rootfs;
        public long createdAtUnix;
        public long updatedAtUnix;
    }

    public static class SandboxListItem {
        public String alias;
        public String clientID;
        public int cpuCount;
        public long diskSizeMB;
        public String endAt;
        public String envdVersion;
        public long memoryMB;
        public Map<String, String> metadata;
        public String sandboxID;
        public String startedAt;
        public String state;
        public String templateID;
        public List<Object> volumeMounts;
    }

    public static class ListOptions {
        private List<String> state = new ArrayList<>();
        private String nextToken = "0";
        private Integer limit = 100;

        public List<String> getState() { return state; }
        public ListOptions setState(List<String> value) { state = value == null ? new ArrayList<>() : new ArrayList<>(value); return this; }
        public ListOptions state(String value) { state.add(value); return this; }
        public String getNextToken() { return nextToken; }
        public ListOptions setNextToken(String value) { nextToken = value == null ? "0" : value; return this; }
        public ListOptions nextToken(String value) { return setNextToken(value); }
        public Integer getLimit() { return limit; }
        public ListOptions setLimit(Integer value) { limit = value; return this; }
        public ListOptions limit(Integer value) { limit = value; return this; }
    }

    public static class SandboxListPage {
        public List<SandboxListItem> items = new ArrayList<>();
        @SerializedName(value = "nextToken", alternate = {"next_token"})
        public String nextToken;
    }

    public static class Snapshot {
        public String snapshotID;
        public String sandboxID;
        public String rootfsPath;
        public String memfilePath;
        public String snapfilePath;
        public long createdAtUnix;
    }

    public static class BalloonConfig {
        public long amountMiB;
        public boolean deflateOnOOM;
        public long statsPollingIntervalS;
        public boolean freePageHinting;
        public boolean freePageReporting;
    }

    public static class BalloonStats {
        public long targetMiB;
        public long actualMiB;
        public long swapIn;
        public long swapOut;
        public long majorFaults;
        public long minorFaults;
        public long freeMemory;
        public long totalMemory;
        public long availableMemory;
        public long diskCaches;
        public long hugetlbAllocations;
        public long hugetlbFailures;
        public long sharedMemory;
        public long unevictableMemory;
        public long oomKill;
        public long allocStall;
        public long asyncScan;
        public long directScan;
        public long asyncReclaim;
        public long directReclaim;
    }

    public static class BalloonHinting {
        public String state;
        public long hostCmd;
        public long guestCmd;
    }

    public static class RuntimeCapabilities {
        public boolean startFromImage;
        public boolean startFromTemplate;
        public boolean startFromSnapshot;
        public boolean pause;
        public boolean resume;
        public boolean fullSnapshot;
        public boolean diffSnapshot;
        public boolean gpu;
        public boolean vsock;
        public boolean tapNetwork;
        public boolean hotplugDisk;
        public boolean hotplugNetwork;
        public boolean liveResizeCPU;
        public boolean liveResizeMemory;
        public boolean gracefulShutdown;
        public boolean serialConsole;
        public boolean jailer;
    }

    public static class RuntimeSummary {
        public String runtimeType;
        public RuntimeCapabilities capabilities;
    }

    public static class RuntimeList {
        public List<RuntimeSummary> runtimes = new ArrayList<>();
    }

    public static class CompatibleTemplateBuild {
        public String buildID;
        public int cpuCount;
        public String createdAt;
        public long diskSizeMB;
        public String envdVersion;
        public String finishedAt;
        public long memoryMB;
        public String status;
        public String updatedAt;
    }

    public static class CompatibleTemplate {
        public String templateID;
        public String buildID;
        public String buildStatus;
        public List<String> aliases = new ArrayList<>();
        public List<String> names = new ArrayList<>();
        @SerializedName("public")
        public boolean isPublic;
        public int spawnCount;
        public int cpuCount;
        public long memoryMB;
        public long diskSizeMB;
        public String envdVersion;
        public String createdAt;
        public String updatedAt;
        public String lastSpawnedAt;
        public TeamUser createdBy;
        public int buildCount;
        public Map<String, String> metadata = new LinkedHashMap<>();
    }

    public static class TeamUser {
        public String email;
        public String id;
    }

    public static class TemplateWithBuilds extends CompatibleTemplate {
        public List<CompatibleTemplateBuild> builds = new ArrayList<>();
    }

    public static class TemplateBuildLogEntry {
        public String level;
        public String message;
        public String timestamp;
        public String step;
    }

    public static class TemplateBuildInfo {
        public String buildID;
        public List<TemplateBuildLogEntry> logEntries = new ArrayList<>();
        public List<String> logs;
        public String status;
        public String templateID;
        public BuildReason reason;
    }

    public static class BuildReason {
        public String message;
        public List<TemplateBuildLogEntry> logEntries = new ArrayList<>();
        public String step;
    }

    public static class TemplateV3 {
        public List<String> aliases = new ArrayList<>();
        public String buildID;
        public Map<String, String> metadata;
        public List<String> names = new ArrayList<>();
        @SerializedName("public")
        public boolean isPublic;
        public List<String> tags = new ArrayList<>();
        public String templateID;
    }

    public static class ConvertResult {
        public String imageID;
        public String templateID;
        public String rootfsPath;
        public long createdAtUnix;
    }

    public static class ImageInfo {
        public String imageID;
        public String rootfsPath;
        public long createdAtUnix;
        public Map<String, String> labels;
    }

    public static class ImageList {
        public List<ImageInfo> images = new ArrayList<>();
    }

    public static class CreateImageRequest {
        public String templateID;
        public CreateImageRequest() {
        }
        public CreateImageRequest(String templateID) { this.templateID = templateID; }
    }

    public static class Strategy {
        public String id;
        public String name;
        public String type;
        public boolean enabled;
        public int priority;
        public Object rules;
        public Object fallbacks;
        public String description;
        public long createdAt;
        public long updatedAt;
    }

    public static class StrategyList {
        public List<Strategy> strategies = new ArrayList<>();
    }

    public static class DeleteStrategyResult {
        public String deleted;
    }

    public static class DefaultStrategyResult {
        public String defaultStrategy;
    }

    public static class Quota {
        public String provider;
        public String region;
        public long total;
        public long used;
        public long available;
        public long updatedAt;
    }

    public static class QuotaList {
        public List<Quota> quotas = new ArrayList<>();
    }

    public static class Decision {
        public String provider;
        public String strategy;
        public String reason;
        public Object fallbacks;
        public double score;
    }

    public static class DecisionRequirements {
        @SerializedName("MinCPU")
        public Integer minCpu;
        @SerializedName("MinMemory")
        public Integer minMemory;
        @SerializedName("NeedGPU")
        public Boolean needGpu;
        @SerializedName("Region")
        public String region;
    }

    public static class DecideRequest {
        @SerializedName("SandboxID")
        public String sandboxID;
        @SerializedName("TemplateID")
        public String templateID;
        @SerializedName("ImageID")
        public String imageID;
        @SerializedName("Labels")
        public Map<String, String> labels;
        @SerializedName("Annotations")
        public Map<String, String> annotations;
        @SerializedName("Region")
        public String region;
        @SerializedName("ProviderHint")
        public String providerHint;
        @SerializedName("Requirements")
        public DecisionRequirements requirements;
    }
}
