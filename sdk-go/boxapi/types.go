package boxapi

const DefaultBaseURL = "http://127.0.0.1:8080"

type HealthStatus struct {
	Status            string `json:"status"`
	Service           string `json:"service,omitempty"`
	StartedAtUnixNano int64  `json:"startedAtUnixNano,omitempty"`
}

type RootfsSource struct {
	Provider string `json:"provider"`
	Image    string `json:"image"`
	PullMode string `json:"pullMode,omitempty"`
}

type OverlaybdRootfs = RootfsSource

type RootfsInfo struct {
	Provider string `json:"provider"`
	Image    string `json:"image,omitempty"`
	PullMode string `json:"pullMode,omitempty"`
	Path     string `json:"path,omitempty"`
}

type CreateSandboxRequest struct {
	TemplateID  string            `json:"templateID,omitempty"`
	ImageID     string            `json:"image_id,omitempty"`
	SnapshotID  string            `json:"snapshot_id,omitempty"`
	Rootfs      *RootfsSource     `json:"rootfs,omitempty"`
	RuntimeType string            `json:"runtime_type,omitempty"`
	GPU         uint32            `json:"gpu,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
	Timeout     int32             `json:"timeout,omitempty"`
}

type Sandbox struct {
	Alias              *string `json:"alias"`
	ClientID           string  `json:"clientID"`
	Domain             *string `json:"domain"`
	EnvdAccessToken    string  `json:"envdAccessToken,omitempty"`
	EnvdVersion        string  `json:"envdVersion"`
	SandboxID          string  `json:"sandboxID"`
	TemplateID         string  `json:"templateID"`
	TrafficAccessToken *string `json:"trafficAccessToken"`
}

type SandboxInfo struct {
	SandboxID     string            `json:"sandboxID"`
	TemplateID    string            `json:"templateID,omitempty"`
	ImageID       string            `json:"imageID,omitempty"`
	SnapshotID    string            `json:"snapshotID,omitempty"`
	State         string            `json:"state"`
	RuntimeType   string            `json:"runtimeType"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Rootfs        *RootfsInfo       `json:"rootfs,omitempty"`
	CreatedAtUnix int64             `json:"createdAtUnix,omitempty"`
	UpdatedAtUnix int64             `json:"updatedAtUnix,omitempty"`
}

type SandboxListItem struct {
	Alias        *string           `json:"alias,omitempty"`
	ClientID     string            `json:"clientID"`
	CPUCount     int               `json:"cpuCount"`
	DiskSizeMB   int64             `json:"diskSizeMB"`
	EndAt        string            `json:"endAt"`
	EnvdVersion  string            `json:"envdVersion"`
	MemoryMB     int64             `json:"memoryMB"`
	Metadata     map[string]string `json:"metadata"`
	SandboxID    string            `json:"sandboxID"`
	StartedAt    string            `json:"startedAt"`
	State        string            `json:"state"`
	TemplateID   string            `json:"templateID"`
	VolumeMounts []any             `json:"volumeMounts"`
}

type ListOptions struct {
	State     []string
	NextToken int
	Limit     int
}

type SandboxListPage struct {
	Items     []SandboxListItem `json:"items"`
	NextToken string            `json:"nextToken,omitempty"`
}

type Snapshot struct {
	SnapshotID    string `json:"snapshotID"`
	SandboxID     string `json:"sandboxID"`
	RootfsPath    string `json:"rootfsPath,omitempty"`
	MemfilePath   string `json:"memfilePath,omitempty"`
	SnapfilePath  string `json:"snapfilePath,omitempty"`
	CreatedAtUnix int64  `json:"createdAtUnix,omitempty"`
}

type BalloonConfig struct {
	AmountMiB             uint32 `json:"amountMiB"`
	DeflateOnOOM          bool   `json:"deflateOnOOM"`
	StatsPollingIntervalS uint32 `json:"statsPollingIntervalS"`
	FreePageHinting       bool   `json:"freePageHinting"`
	FreePageReporting     bool   `json:"freePageReporting"`
}

type BalloonStats struct {
	TargetMiB          uint32 `json:"targetMiB"`
	ActualMiB          uint32 `json:"actualMiB"`
	SwapIn             uint64 `json:"swapIn"`
	SwapOut            uint64 `json:"swapOut"`
	MajorFaults        uint64 `json:"majorFaults"`
	MinorFaults        uint64 `json:"minorFaults"`
	FreeMemory         uint64 `json:"freeMemory"`
	TotalMemory        uint64 `json:"totalMemory"`
	AvailableMemory    uint64 `json:"availableMemory"`
	DiskCaches         uint64 `json:"diskCaches"`
	HugetlbAllocations uint64 `json:"hugetlbAllocations"`
	HugetlbFailures    uint64 `json:"hugetlbFailures"`
	SharedMemory       uint64 `json:"sharedMemory"`
	UnevictableMemory  uint64 `json:"unevictableMemory"`
	OOMKill            uint64 `json:"oomKill"`
	AllocStall         uint64 `json:"allocStall"`
	AsyncScan          uint64 `json:"asyncScan"`
	DirectScan         uint64 `json:"directScan"`
	AsyncReclaim       uint64 `json:"asyncReclaim"`
	DirectReclaim      uint64 `json:"directReclaim"`
}

type BalloonHinting struct {
	State    string `json:"state"`
	HostCmd  uint32 `json:"hostCmd"`
	GuestCmd uint32 `json:"guestCmd,omitempty"`
}

type RuntimeCapabilities struct {
	StartFromImage    bool `json:"startFromImage"`
	StartFromTemplate bool `json:"startFromTemplate"`
	StartFromSnapshot bool `json:"startFromSnapshot"`
	Pause             bool `json:"pause"`
	Resume            bool `json:"resume"`
	FullSnapshot      bool `json:"fullSnapshot"`
	DiffSnapshot      bool `json:"diffSnapshot"`
	GPU               bool `json:"gpu"`
	Vsock             bool `json:"vsock"`
	TapNetwork        bool `json:"tapNetwork"`
	HotplugDisk       bool `json:"hotplugDisk"`
	HotplugNetwork    bool `json:"hotplugNetwork"`
	LiveResizeCPU     bool `json:"liveResizeCPU"`
	LiveResizeMemory  bool `json:"liveResizeMemory"`
	GracefulShutdown  bool `json:"gracefulShutdown"`
	SerialConsole     bool `json:"serialConsole"`
	Jailer            bool `json:"jailer"`
}

type RuntimeSummary struct {
	RuntimeType  string              `json:"runtimeType"`
	Capabilities RuntimeCapabilities `json:"capabilities"`
}

type RuntimeList struct {
	Runtimes []RuntimeSummary `json:"runtimes"`
}

type CompatibleTemplateBuild struct {
	BuildID     string `json:"buildID"`
	CPUCount    int    `json:"cpuCount"`
	CreatedAt   string `json:"createdAt"`
	DiskSizeMB  int64  `json:"diskSizeMB,omitempty"`
	EnvdVersion string `json:"envdVersion,omitempty"`
	FinishedAt  string `json:"finishedAt,omitempty"`
	MemoryMB    int64  `json:"memoryMB"`
	Status      string `json:"status"`
	UpdatedAt   string `json:"updatedAt"`
}

type CompatibleTemplate struct {
	TemplateID    string            `json:"templateID"`
	BuildID       string            `json:"buildID"`
	BuildStatus   string            `json:"buildStatus"`
	Aliases       []string          `json:"aliases"`
	Names         []string          `json:"names"`
	Public        bool              `json:"public"`
	SpawnCount    int               `json:"spawnCount"`
	CPUCount      int               `json:"cpuCount"`
	MemoryMB      int64             `json:"memoryMB"`
	DiskSizeMB    int64             `json:"diskSizeMB"`
	EnvdVersion   string            `json:"envdVersion"`
	CreatedAt     string            `json:"createdAt"`
	UpdatedAt     string            `json:"updatedAt"`
	LastSpawnedAt string            `json:"lastSpawnedAt"`
	CreatedBy     *TeamUser         `json:"createdBy"`
	BuildCount    int               `json:"buildCount"`
	Metadata      map[string]string `json:"metadata"`
}

type TeamUser struct {
	Email string `json:"email,omitempty"`
	ID    string `json:"id,omitempty"`
}

type TemplateWithBuilds struct {
	CompatibleTemplate
	Builds []CompatibleTemplateBuild `json:"builds"`
}

type TemplateBuildLogEntry struct {
	Level     string `json:"level"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	Step      string `json:"step,omitempty"`
}

type TemplateBuildInfo struct {
	BuildID    string                  `json:"buildID"`
	LogEntries []TemplateBuildLogEntry `json:"logEntries"`
	Logs       []string                `json:"logs"`
	Status     string                  `json:"status"`
	TemplateID string                  `json:"templateID"`
	Reason     *BuildReason            `json:"reason,omitempty"`
}

type BuildReason struct {
	Message    string                  `json:"message,omitempty"`
	LogEntries []TemplateBuildLogEntry `json:"logEntries,omitempty"`
	Step       string                  `json:"step,omitempty"`
}

type TemplateV3 struct {
	Aliases    []string          `json:"aliases"`
	BuildID    string            `json:"buildID"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Names      []string          `json:"names"`
	Public     bool              `json:"public"`
	Tags       []string          `json:"tags"`
	TemplateID string            `json:"templateID"`
}

type ConvertResult struct {
	ImageID       string `json:"imageID"`
	TemplateID    string `json:"templateID"`
	RootfsPath    string `json:"rootfsPath"`
	CreatedAtUnix int64  `json:"createdAtUnix"`
}

type ImageInfo struct {
	ImageID       string            `json:"imageID"`
	RootfsPath    string            `json:"rootfsPath"`
	CreatedAtUnix int64             `json:"createdAtUnix"`
	Labels        map[string]string `json:"labels,omitempty"`
}

type ImageList struct {
	Images []ImageInfo `json:"images"`
}

type CreateImageRequest struct {
	TemplateID string `json:"templateID"`
}

type Strategy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Enabled     bool   `json:"enabled"`
	Priority    int    `json:"priority"`
	Rules       any    `json:"rules"`
	Fallbacks   any    `json:"fallbacks"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

type StrategyList struct {
	Strategies []Strategy `json:"strategies"`
}

type DeleteStrategyResult struct {
	Deleted string `json:"deleted"`
}

type DefaultStrategyResult struct {
	DefaultStrategy string `json:"defaultStrategy"`
}

type Quota struct {
	Provider  string `json:"provider"`
	Region    string `json:"region"`
	Total     int64  `json:"total"`
	Used      int64  `json:"used"`
	Available int64  `json:"available"`
	UpdatedAt int64  `json:"updatedAt"`
}

type QuotaList struct {
	Quotas []Quota `json:"quotas"`
}

type Decision struct {
	Provider  string  `json:"provider"`
	Strategy  string  `json:"strategy"`
	Reason    string  `json:"reason"`
	Fallbacks any     `json:"fallbacks"`
	Score     float64 `json:"score"`
}

type DecisionRequirements struct {
	MinCPU    int    `json:"MinCPU,omitempty"`
	MinMemory int    `json:"MinMemory,omitempty"`
	NeedGPU   bool   `json:"NeedGPU,omitempty"`
	Region    string `json:"Region,omitempty"`
}

type DecideRequest struct {
	SandboxID    string                `json:"SandboxID,omitempty"`
	TemplateID   string                `json:"TemplateID,omitempty"`
	ImageID      string                `json:"ImageID,omitempty"`
	Labels       map[string]string     `json:"Labels,omitempty"`
	Annotations  map[string]string     `json:"Annotations,omitempty"`
	Region       string                `json:"Region,omitempty"`
	ProviderHint string                `json:"ProviderHint,omitempty"`
	Requirements *DecisionRequirements `json:"Requirements,omitempty"`
}
