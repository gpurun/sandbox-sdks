package boxd

import "encoding/base64"

const DefaultBaseURL = "http://127.0.0.1:8082"

const SandboxIDHeader = "Novita-Sandbox-Id"

const ConnectContentType = "application/connect+json"

type HealthStatus struct {
	Status            string `json:"status"`
	Service           string `json:"service,omitempty"`
	StartedAtUnixNano int64  `json:"startedAtUnixNano,omitempty"`
}

type ExecResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type ProcessInfo struct {
	ID        string   `json:"id,omitempty"`
	PID       int      `json:"pid,omitempty"`
	Tag       string   `json:"tag,omitempty"`
	Cmd       []string `json:"cmd,omitempty"`
	Args      []string `json:"args,omitempty"`
	Cwd       string   `json:"cwd,omitempty"`
	Tty       bool     `json:"tty,omitempty"`
	State     string   `json:"state,omitempty"`
	StartedAt string   `json:"startedAt,omitempty"`
	Rows      int      `json:"rows,omitempty"`
	Cols      int      `json:"cols,omitempty"`
}

type StartProcessRequest struct {
	Cmd  []string          `json:"cmd"`
	Cwd  string            `json:"cwd,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
	Tty  bool              `json:"tty,omitempty"`
	Rows int               `json:"rows,omitempty"`
	Cols int               `json:"cols,omitempty"`
	Tag  string            `json:"tag,omitempty"`
}

type ProcessExit struct {
	ExitCode int    `json:"exitCode"`
	Exited   bool   `json:"exited"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type ProcessStreamConfig struct {
	Cmd  string            `json:"cmd"`
	Args []string          `json:"args,omitempty"`
	Envs map[string]string `json:"envs,omitempty"`
	Cwd  string            `json:"cwd,omitempty"`
}

type ProcessSize struct {
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
}

type ProcessSelector struct {
	PID int    `json:"pid,omitempty"`
	Tag string `json:"tag,omitempty"`
}

type ConnectProcessEntry struct {
	Config ProcessStreamConfig `json:"config"`
	PID    int                 `json:"pid"`
	Tag    string              `json:"tag,omitempty"`
}

type StartProcessStreamRequest struct {
	Process ProcessStreamConfig `json:"process"`
	Pty     *ProcessPTY         `json:"pty,omitempty"`
	Tag     string              `json:"tag,omitempty"`
	Stdin   bool                `json:"stdin,omitempty"`
}

type ConnectProcessStreamRequest struct {
	Process ProcessSelector `json:"process"`
}

type ProcessPTY struct {
	Size *ProcessSize `json:"size,omitempty"`
}

type UpdateProcessRequest struct {
	Process ProcessSelector `json:"process"`
	Pty     *ProcessPTY     `json:"pty,omitempty"`
}

type SendInputRequest struct {
	Process ProcessSelector  `json:"process"`
	Input   SendInputPayload `json:"input"`
}

type SendInputPayload struct {
	Stdin string `json:"stdin,omitempty"`
	Pty   string `json:"pty,omitempty"`
}

type SendSignalRequest struct {
	Process ProcessSelector `json:"process"`
	Signal  int             `json:"signal"`
}

type ProcessStartEvent struct {
	PID int `json:"pid"`
}

// ProcessDataEvent carries one chunk of process output from the Connect
// stream. The fields are standard base64 strings because the Connect
// envelope is JSON and cannot carry raw bytes; decode them before use,
// or call StdoutBytes/StderrBytes/PtyBytes.
type ProcessDataEvent struct {
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
	Pty    string `json:"pty,omitempty"`
}

// StdoutBytes base64-decodes the stdout field.
func (e ProcessDataEvent) StdoutBytes() ([]byte, error) { return decodeDataBase64(e.Stdout) }

// StderrBytes base64-decodes the stderr field.
func (e ProcessDataEvent) StderrBytes() ([]byte, error) { return decodeDataBase64(e.Stderr) }

// PtyBytes base64-decodes the pty field.
func (e ProcessDataEvent) PtyBytes() ([]byte, error) { return decodeDataBase64(e.Pty) }

func decodeDataBase64(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

type ProcessEventBody struct {
	Start     *ProcessStartEvent `json:"start,omitempty"`
	Data      *ProcessDataEvent  `json:"data,omitempty"`
	Keepalive map[string]any     `json:"keepalive,omitempty"`
	End       *ProcessExit       `json:"end,omitempty"`
}

type ProcessEvent struct {
	Event ProcessEventBody `json:"event"`
}

// ProcessWSEvent is a frame from the process WebSocket connect endpoint.
// boxd sends {"type":"start","process":{...}} and {"type":"end","exit":{...}}
// as TEXT JSON frames; boxproxy rewrites them to BINARY, which the SDK
// tolerates by probing.
type ProcessWSEvent struct {
	Type    string       `json:"type"`
	Process *ProcessInfo `json:"process,omitempty"`
	Exit    *ProcessExit `json:"exit,omitempty"`
}
