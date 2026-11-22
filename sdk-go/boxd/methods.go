package boxd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) Health() (*HealthStatus, error) {
	var out HealthStatus
	if _, _, err := c.requestWithoutSandbox(http.MethodGet, "/healthz", &out, "application/json"); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Exec(cmd []string, envVars map[string]string) (*ExecResult, error) {
	if envVars == nil {
		envVars = map[string]string{}
	}
	body := map[string]any{"cmd": cmd, "envVars": envVars}
	status, data, err := c.request(http.MethodPost, "/exec", body, nil, "application/json", http.StatusUnprocessableEntity)
	if err != nil {
		return nil, err
	}
	return parseExecResult(status, data), nil
}

func (c *Client) ListProcesses() ([]ProcessInfo, error) {
	var raw json.RawMessage
	if _, _, err := c.request(http.MethodGet, "/processes", nil, &raw, "application/json"); err != nil {
		return nil, err
	}
	var list []ProcessInfo
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var envelope struct {
		Processes []ProcessInfo `json:"processes"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, &Error{Status: 0, Code: "invalid_response", Message: "invalid JSON response", Err: err}
	}
	return envelope.Processes, nil
}

func (c *Client) StartProcess(req StartProcessRequest) (*ProcessInfo, error) {
	var raw json.RawMessage
	if _, _, err := c.request(http.MethodPost, "/processes", req, &raw, "application/json", http.StatusCreated); err != nil {
		return nil, err
	}
	return decodeProcess(raw)
}

func (c *Client) WaitProcess(id string) (*ProcessInfo, error) {
	var raw json.RawMessage
	path := "/processes/" + url.PathEscape(id) + "/wait"
	if _, _, err := c.request(http.MethodGet, path, nil, &raw, "application/json"); err != nil {
		return nil, err
	}
	return decodeProcess(raw)
}

func (c *Client) ResizeProcess(id string, rows, cols int) error {
	body := map[string]int{"rows": rows, "cols": cols}
	path := "/processes/" + url.PathEscape(id) + "/resize"
	_, _, err := c.request(http.MethodPost, path, body, nil, "application/json")
	return err
}

func (c *Client) SignalProcess(id string, signal string) error {
	body := map[string]string{"signal": signal}
	path := "/processes/" + url.PathEscape(id) + "/signal"
	_, _, err := c.request(http.MethodPost, path, body, nil, "application/json")
	return err
}

func (c *Client) ListProcessesConnect() ([]ConnectProcessEntry, error) {
	var envelope struct {
		Processes []ConnectProcessEntry `json:"processes"`
	}
	if _, _, err := c.request(http.MethodPost, "/process.Process/List", map[string]any{}, &envelope, ConnectContentType); err != nil {
		return nil, err
	}
	return envelope.Processes, nil
}

func (c *Client) StartProcessStream(req StartProcessStreamRequest, onEvent func(ProcessEvent)) (*ProcessStream, error) {
	res, err := c.requestStream(http.MethodPost, "/process.Process/Start", req, ConnectContentType)
	if err != nil {
		return nil, err
	}
	return newProcessStream(res, onEvent), nil
}

func (c *Client) ConnectProcessStream(req ConnectProcessStreamRequest, onEvent func(ProcessEvent)) (*ProcessStream, error) {
	res, err := c.requestStream(http.MethodPost, "/process.Process/Connect", req, ConnectContentType)
	if err != nil {
		return nil, err
	}
	return newProcessStream(res, onEvent), nil
}

func (c *Client) UpdateProcess(req UpdateProcessRequest) error {
	_, _, err := c.request(http.MethodPost, "/process.Process/Update", req, nil, ConnectContentType)
	return err
}

func (c *Client) SendInput(req SendInputRequest) error {
	req.Input.Stdin = encodeInput(req.Input.Stdin)
	req.Input.Pty = encodeInput(req.Input.Pty)
	_, _, err := c.request(http.MethodPost, "/process.Process/SendInput", req, nil, ConnectContentType)
	return err
}

func encodeInput(value string) string {
	if value == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(value))
}

func (c *Client) SendSignal(req SendSignalRequest) error {
	_, _, err := c.request(http.MethodPost, "/process.Process/SendSignal", req, nil, ConnectContentType)
	return err
}

func decodeProcess(raw json.RawMessage) (*ProcessInfo, error) {
	var envelope struct {
		Process *ProcessInfo `json:"process"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Process != nil {
		return envelope.Process, nil
	}
	var process ProcessInfo
	if err := json.Unmarshal(raw, &process); err != nil {
		return nil, &Error{Status: 0, Code: "invalid_response", Message: "invalid JSON response", Err: err}
	}
	return &process, nil
}

func parseExecResult(status int, data []byte) *ExecResult {
	defaultExit := 0
	if status == http.StatusUnprocessableEntity {
		defaultExit = 1
	}
	result := &ExecResult{ExitCode: defaultExit}
	text := stringsTrimSpace(string(data))
	if text == "" {
		return result
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		result.Stdout = text
		return result
	}
	if value, ok := firstJSONNumber(fields, "exitCode", "exit_code", "code"); ok {
		result.ExitCode = value
	}
	if value, ok := firstJSONString(fields, "stdout", "output"); ok {
		result.Stdout = value
	}
	if value, ok := firstJSONString(fields, "stderr", "error"); ok {
		result.Stderr = value
	}
	return result
}

func firstJSONNumber(fields map[string]json.RawMessage, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value int
		if err := json.Unmarshal(raw, &value); err == nil {
			return value, true
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			if parsed, err := strconv.Atoi(stringsTrimSpace(text)); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func firstJSONString(fields map[string]json.RawMessage, keys ...string) (string, bool) {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			return value, true
		}
	}
	return "", false
}

func stringsTrimSpace(value string) string {
	return strings.TrimSpace(value)
}
