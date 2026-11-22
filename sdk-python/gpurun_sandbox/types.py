from __future__ import annotations

from typing import Any, Dict, List, Optional, TypedDict

JsonRecord = Dict[str, Any]
JsonValue = Any


class OverlaybdRootfs(TypedDict, total=False):
    provider: str
    image: str
    pullMode: str


class RootfsInfo(TypedDict, total=False):
    provider: str
    image: str
    pullMode: str
    path: str


class CreateSandboxRequest(TypedDict, total=False):
    templateID: str
    imageID: str
    snapshotID: str
    rootfs: OverlaybdRootfs
    timeout: int
    metadata: Dict[str, str]
    envVars: Dict[str, str]


class Sandbox(TypedDict, total=False):
    alias: Optional[str]
    clientID: str
    domain: Optional[str]
    envdAccessToken: str
    envdVersion: str
    sandboxID: str
    templateID: str
    trafficAccessToken: Optional[str]


class SandboxInfo(TypedDict, total=False):
    sandboxID: str
    templateID: str
    imageID: str
    snapshotID: str
    state: str
    runtimeType: str
    metadata: Dict[str, str]
    rootfs: RootfsInfo
    createdAtUnix: int
    updatedAtUnix: int


class SandboxListItem(TypedDict, total=False):
    alias: Optional[str]
    clientID: str
    cpuCount: int
    diskSizeMB: int
    endAt: str
    envdVersion: str
    memoryMB: int
    metadata: Dict[str, str]
    sandboxID: str
    startedAt: str
    state: str
    templateID: str
    volumeMounts: List[Any]


class ListSandboxesV2Options(TypedDict, total=False):
    state: Any
    nextToken: Any
    limit: int


class ListSandboxesV2Result(TypedDict, total=False):
    items: List[SandboxListItem]
    next_token: Optional[str]


class Snapshot(TypedDict, total=False):
    snapshotID: str
    sandboxID: str
    rootfsPath: str
    memfilePath: str
    snapfilePath: str
    createdAtUnix: int


class BalloonConfig(TypedDict, total=False):
    amountMiB: int
    deflateOnOOM: bool
    statsPollingIntervalS: int
    freePageHinting: bool
    freePageReporting: bool


class BalloonStats(TypedDict, total=False):
    targetMiB: int
    actualMiB: int
    swapIn: int
    swapOut: int
    majorFaults: int
    minorFaults: int
    freeMemory: int
    totalMemory: int
    availableMemory: int
    diskCaches: int
    hugetlbAllocations: int
    hugetlbFailures: int
    sharedMemory: int
    unevictableMemory: int
    oomKill: int
    allocStall: int
    asyncScan: int
    directScan: int
    asyncReclaim: int
    directReclaim: int


class BalloonHinting(TypedDict, total=False):
    state: str
    hostCmd: int
    guestCmd: int


class RuntimeCapabilities(TypedDict, total=False):
    startFromImage: bool
    startFromTemplate: bool
    startFromSnapshot: bool
    pause: bool
    resume: bool
    fullSnapshot: bool
    diffSnapshot: bool
    gpu: bool
    vsock: bool
    tapNetwork: bool
    hotplugDisk: bool
    hotplugNetwork: bool
    liveResizeCPU: bool
    liveResizeMemory: bool
    gracefulShutdown: bool
    serialConsole: bool
    jailer: bool


class RuntimeSummary(TypedDict, total=False):
    runtimeType: str
    capabilities: RuntimeCapabilities


class CompatibleTemplateBuild(TypedDict, total=False):
    buildID: str
    cpuCount: int
    createdAt: str
    diskSizeMB: int
    envdVersion: str
    finishedAt: str
    memoryMB: int
    status: str
    updatedAt: str


class CompatibleTemplate(TypedDict, total=False):
    templateID: str
    buildID: str
    buildStatus: str
    aliases: List[str]
    names: List[str]
    public: bool
    spawnCount: int
    cpuCount: int
    memoryMB: int
    diskSizeMB: int
    envdVersion: str
    createdAt: str
    updatedAt: str
    lastSpawnedAt: str
    createdBy: str
    buildCount: int
    metadata: Dict[str, str]


class TemplateWithBuilds(CompatibleTemplate, total=False):
    builds: List[CompatibleTemplateBuild]


class TemplateBuildLogEntry(TypedDict, total=False):
    level: str
    message: str
    timestamp: str
    step: str


class TemplateBuildInfo(TypedDict, total=False):
    buildID: str
    logEntries: List[TemplateBuildLogEntry]
    logs: List[str]
    status: str
    templateID: str
    reason: str


class TemplateV3(TypedDict, total=False):
    aliases: List[str]
    buildID: str
    metadata: Dict[str, str]
    names: List[str]
    public: bool
    tags: List[str]
    templateID: str


class ConvertResult(TypedDict, total=False):
    imageID: str
    templateID: str
    rootfsPath: str
    createdAtUnix: int


class ImageInfo(TypedDict, total=False):
    imageID: str
    rootfsPath: str
    createdAtUnix: int
    labels: Dict[str, str]


class CreateImageRequest(TypedDict, total=False):
    templateID: str


class Strategy(TypedDict, total=False):
    id: str
    name: str
    type: str
    enabled: bool
    priority: int
    rules: Any
    fallbacks: Any
    description: str
    createdAt: str
    updatedAt: str


class Quota(TypedDict, total=False):
    provider: str
    region: str
    total: int
    used: int
    available: int
    updatedAt: str


class Decision(TypedDict, total=False):
    provider: str
    strategy: str
    reason: str
    fallbacks: Any
    score: float


class DecideRequirements(TypedDict, total=False):
    minCPU: int
    minMemory: int
    needGPU: bool
    region: str


class DecideRequest(TypedDict, total=False):
    sandboxID: str
    templateID: str
    imageID: str
    labels: Dict[str, str]
    annotations: Dict[str, str]
    region: str
    providerHint: str
    requirements: DecideRequirements


class ExecResult(TypedDict, total=False):
    exitCode: int
    stdout: str
    stderr: str


class ProcessInfo(TypedDict, total=False):
    pid: int
    tag: str
    cmd: List[str]
    args: List[str]
    cwd: str
    tty: bool
    rows: int
    cols: int


class StartProcessRequest(TypedDict, total=False):
    cmd: List[str]
    cwd: str
    env: Dict[str, str]
    tty: bool
    rows: int
    cols: int
    tag: str


class EndEvent(TypedDict, total=False):
    exitCode: int
    exited: bool
    status: str
    error: str


class ProcessStreamConfig(TypedDict, total=False):
    cmd: str
    args: List[str]
    envs: Dict[str, str]
    cwd: str


class ConnectProcessEntry(TypedDict, total=False):
    config: ProcessStreamConfig
    pid: int
    tag: str


class ProcessSelector(TypedDict, total=False):
    pid: int
    tag: str


class ProcessSize(TypedDict, total=False):
    cols: int
    rows: int


class StartProcessStreamRequest(TypedDict, total=False):
    process: ProcessStreamConfig
    pty: Dict[str, Any]
    tag: str
    stdin: bool


class ConnectProcessStreamRequest(TypedDict, total=False):
    process: ProcessSelector


class UpdateProcessRequest(TypedDict, total=False):
    process: ProcessSelector
    pty: Dict[str, Any]


class SendInputRequest(TypedDict, total=False):
    process: ProcessSelector
    input: Dict[str, str]


class SendSignalRequest(TypedDict, total=False):
    process: ProcessSelector
    signal: int


class ProcessEvent(TypedDict, total=False):
    event: JsonRecord


class ClientOptions(TypedDict, total=False):
    baseUrl: str
    apiKey: str
    accessToken: str
    token: str
    timeoutMs: int
    headers: Dict[str, str]


class BoxApiClientOptions(ClientOptions, total=False):
    pass


class BoxdClientOptions(ClientOptions, total=False):
    sandboxId: str


class GpurunOptions(BoxdClientOptions, total=False):
    dataUrl: str


__all__ = [
    "JsonRecord",
    "JsonValue",
    "OverlaybdRootfs",
    "RootfsInfo",
    "CreateSandboxRequest",
    "Sandbox",
    "SandboxInfo",
    "SandboxListItem",
    "ListSandboxesV2Options",
    "ListSandboxesV2Result",
    "Snapshot",
    "BalloonConfig",
    "BalloonStats",
    "BalloonHinting",
    "RuntimeCapabilities",
    "RuntimeSummary",
    "CompatibleTemplateBuild",
    "CompatibleTemplate",
    "TemplateWithBuilds",
    "TemplateBuildLogEntry",
    "TemplateBuildInfo",
    "TemplateV3",
    "ConvertResult",
    "ImageInfo",
    "CreateImageRequest",
    "Strategy",
    "Quota",
    "Decision",
    "DecideRequirements",
    "DecideRequest",
    "ExecResult",
    "ProcessInfo",
    "StartProcessRequest",
    "EndEvent",
    "ProcessStreamConfig",
    "ConnectProcessEntry",
    "ProcessSelector",
    "ProcessSize",
    "StartProcessStreamRequest",
    "ConnectProcessStreamRequest",
    "UpdateProcessRequest",
    "SendInputRequest",
    "SendSignalRequest",
    "ProcessEvent",
    "ClientOptions",
    "BoxApiClientOptions",
    "BoxdClientOptions",
    "GpurunOptions",
]
