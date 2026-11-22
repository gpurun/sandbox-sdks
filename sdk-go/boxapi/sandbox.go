package boxapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) Health() (*HealthStatus, error) {
	var out HealthStatus
	if _, _, err := c.request(http.MethodGet, "/health", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateSandbox(req CreateSandboxRequest) (*Sandbox, error) {
	var out Sandbox
	if _, _, err := c.request(http.MethodPost, "/sandboxes", req, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListSandboxes() ([]SandboxInfo, error) {
	var out []SandboxInfo
	if _, _, err := c.request(http.MethodGet, "/v1/sandboxes", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListSandboxesV2(opts ListOptions) (*SandboxListPage, error) {
	if opts.NextToken < 0 {
		return nil, &Error{Status: 0, Code: "invalid_argument", Message: "nextToken must be a non-negative number"}
	}
	if opts.Limit < 0 {
		return nil, &Error{Status: 0, Code: "invalid_argument", Message: "limit must be a non-negative number"}
	}
	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := url.Values{}
	for _, state := range opts.State {
		query.Add("state", state)
	}
	query.Set("nextToken", strconv.Itoa(opts.NextToken))
	query.Set("limit", strconv.Itoa(limit))

	body, headers, err := c.request(http.MethodGet, "/sandboxes", nil, query, nil)
	if err != nil {
		return nil, err
	}
	page := &SandboxListPage{}
	if len(body) > 0 {
		if err := jsonUnmarshal(body, &page.Items); err != nil {
			var wire struct {
				Items          []SandboxListItem `json:"items"`
				NextToken      string            `json:"nextToken"`
				NextTokenSnake string            `json:"next_token"`
			}
			if wireErr := jsonUnmarshal(body, &wire); wireErr != nil {
				return nil, &Error{Status: http.StatusOK, Code: "invalid_response", Message: "invalid JSON response", Err: wireErr}
			}
			page.Items = wire.Items
			page.NextToken = firstNonEmpty(wire.NextToken, wire.NextTokenSnake)
		}
	}
	if token := headers.Get("X-Next-Token"); token != "" {
		page.NextToken = token
	}
	return page, nil
}

func (c *Client) GetSandbox(id string) (*SandboxInfo, error) {
	var out SandboxInfo
	if _, _, err := c.request(http.MethodGet, "/sandboxes/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) KillSandbox(id string) error {
	_, _, err := c.request(http.MethodDelete, "/sandboxes/"+url.PathEscape(id), nil, nil, nil)
	return err
}

func (c *Client) PauseSandbox(id string) (*Snapshot, error) {
	var out Snapshot
	if _, _, err := c.request(http.MethodPost, "/v1/sandboxes/"+url.PathEscape(id)+"/pause", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ResumeSandbox(id string) (*SandboxInfo, error) {
	var out SandboxInfo
	if _, _, err := c.request(http.MethodPost, "/v1/sandboxes/"+url.PathEscape(id)+"/resume", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ConnectSandbox(id string) (*Sandbox, error) {
	var out Sandbox
	if _, _, err := c.request(http.MethodPost, "/sandboxes/"+url.PathEscape(id)+"/connect", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SetSandboxTimeout(id string) error {
	_, _, err := c.request(http.MethodPost, "/sandboxes/"+url.PathEscape(id)+"/timeout", nil, nil, nil)
	return err
}

func (c *Client) PoweroffSandbox(id string) (*SandboxInfo, error) {
	var out SandboxInfo
	if _, _, err := c.request(http.MethodPost, "/v1/sandboxes/"+url.PathEscape(id)+"/poweroff", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PoweronSandbox(id string) (*SandboxInfo, error) {
	var out SandboxInfo
	if _, _, err := c.request(http.MethodPost, "/v1/sandboxes/"+url.PathEscape(id)+"/poweron", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RebootSandbox(id string) (*SandboxInfo, error) {
	var out SandboxInfo
	if _, _, err := c.request(http.MethodPost, "/v1/sandboxes/"+url.PathEscape(id)+"/reboot", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetBalloon(id string) (*BalloonConfig, error) {
	var out BalloonConfig
	if _, _, err := c.request(http.MethodGet, "/v1/sandboxes/"+url.PathEscape(id)+"/balloon", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateBalloon(id string, amountMiB uint32) (*BalloonConfig, error) {
	var out BalloonConfig
	body := map[string]any{"amountMiB": amountMiB}
	if _, _, err := c.request(http.MethodPatch, "/v1/sandboxes/"+url.PathEscape(id)+"/balloon", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetBalloonStats(id string) (*BalloonStats, error) {
	var out BalloonStats
	if _, _, err := c.request(http.MethodGet, "/v1/sandboxes/"+url.PathEscape(id)+"/balloon/statistics", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateBalloonStats(id string, intervalS uint32) (*BalloonConfig, error) {
	var out BalloonConfig
	body := map[string]any{"statsPollingIntervalS": intervalS}
	if _, _, err := c.request(http.MethodPatch, "/v1/sandboxes/"+url.PathEscape(id)+"/balloon/statistics", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetBalloonHinting(id string) (*BalloonHinting, error) {
	var out BalloonHinting
	if _, _, err := c.request(http.MethodGet, "/v1/sandboxes/"+url.PathEscape(id)+"/balloon/hinting", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StartBalloonHinting(id string, acknowledgeOnStop *bool) (*BalloonHinting, error) {
	var out BalloonHinting
	value := true
	if acknowledgeOnStop != nil {
		value = *acknowledgeOnStop
	}
	body := map[string]any{"acknowledgeOnStop": value}
	if _, _, err := c.request(http.MethodPost, "/v1/sandboxes/"+url.PathEscape(id)+"/balloon/hinting/start", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StopBalloonHinting(id string) (*BalloonHinting, error) {
	var out BalloonHinting
	if _, _, err := c.request(http.MethodPost, "/v1/sandboxes/"+url.PathEscape(id)+"/balloon/hinting/stop", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func jsonUnmarshal(data []byte, out any) error {
	return json.Unmarshal(data, out)
}
