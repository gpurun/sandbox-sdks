package boxd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Error struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return fmt.Sprintf("boxd: %s (status %d, code %s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("boxd: %s (status %d)", e.Message, e.Status)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type Client struct {
	BaseURL     string
	SandboxID   string
	Token       string
	APIKey      string
	AccessToken string
	Timeout     time.Duration
	HTTPClient  *http.Client
	Headers     map[string]string
}

type ClientOptions struct {
	BaseURL     string
	SandboxID   string
	Token       string
	APIKey      string
	AccessToken string
	Timeout     time.Duration
	HTTPClient  *http.Client
	Headers     map[string]string
}

func NewClient(opts ClientOptions) (*Client, error) {
	return &Client{
		BaseURL:     firstNonEmpty(opts.BaseURL, envFirst("GPURUN_BOXD_URL"), DefaultBaseURL),
		SandboxID:   firstNonEmpty(opts.SandboxID, envFirst("GPURUN_SANDBOX_ID", "NOVITA_SANDBOX_ID")),
		Token:       opts.Token,
		APIKey:      opts.APIKey,
		AccessToken: opts.AccessToken,
		Timeout:     opts.Timeout,
		HTTPClient:  opts.HTTPClient,
		Headers:     cloneHeaders(opts.Headers),
	}, nil
}

func New(opts ...ClientOptions) *Client {
	var options ClientOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	client, _ := NewClient(options)
	return client
}

func (c *Client) request(method, path string, body any, out any, contentType string, allowed ...int) (int, []byte, error) {
	return c.requestWithContext(context.Background(), method, path, body, out, contentType, allowed...)
}

func (c *Client) requestWithContext(ctx context.Context, method, path string, body any, out any, contentType string, allowed ...int) (int, []byte, error) {
	return c.doRequest(ctx, method, path, body, out, contentType, true, allowed...)
}

func (c *Client) requestWithoutSandbox(method, path string, out any, contentType string) (int, []byte, error) {
	return c.doRequest(context.Background(), method, path, nil, out, contentType, false)
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any, out any, contentType string, requireSandbox bool, allowed ...int) (int, []byte, error) {
	target, sandboxID, err := c.targetWith(path, requireSandbox)
	if err != nil {
		return 0, nil, err
	}
	if contentType == "" {
		contentType = "application/json"
	}

	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return 0, nil, &Error{Status: 0, Code: "invalid_request", Message: err.Error(), Err: err}
		}
	}

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return 0, nil, &Error{Status: 0, Code: "invalid_request", Message: err.Error(), Err: err}
	}
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", contentType)
	if sandboxID != "" {
		req.Header.Set(SandboxIDHeader, sandboxID)
	}
	if payload != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if token := c.bearerToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	res, err := c.httpClient().Do(req)
	if err != nil {
		return 0, nil, &Error{Status: 0, Code: "network_error", Message: err.Error(), Err: err}
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, nil, &Error{Status: res.StatusCode, Code: "network_error", Message: err.Error(), Err: err}
	}
	if !successStatus(res.StatusCode) && !allowedStatus(res.StatusCode, allowed...) {
		return res.StatusCode, data, decodeError(res.StatusCode, data)
	}
	if out != nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return res.StatusCode, data, &Error{Status: res.StatusCode, Code: "invalid_response", Message: "invalid JSON response", Err: err}
		}
	}
	return res.StatusCode, data, nil
}

func (c *Client) requestStream(method, path string, body any, contentType string) (*http.Response, error) {
	target, sandboxID, err := c.target(path)
	if err != nil {
		return nil, err
	}
	if contentType == "" {
		contentType = ConnectContentType
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &Error{Status: 0, Code: "invalid_request", Message: err.Error(), Err: err}
	}
	req, err := http.NewRequestWithContext(context.Background(), method, target.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, &Error{Status: 0, Code: "invalid_request", Message: err.Error(), Err: err}
	}
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", contentType)
	req.Header.Set(SandboxIDHeader, sandboxID)
	req.Header.Set("Content-Type", contentType)
	if token := c.bearerToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, &Error{Status: 0, Code: "network_error", Message: err.Error(), Err: err}
	}
	if !successStatus(res.StatusCode) {
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		return nil, decodeError(res.StatusCode, data)
	}
	return res, nil
}

func (c *Client) target(path string) (*url.URL, string, error) {
	return c.targetWith(path, true)
}

func (c *Client) targetWith(path string, requireSandbox bool) (*url.URL, string, error) {
	sandboxID := strings.TrimSpace(firstNonEmpty(c.SandboxID,
		envFirst("GPURUN_SANDBOX_ID", "NOVITA_SANDBOX_ID")))
	if requireSandbox && sandboxID == "" {
		return nil, "", &Error{Status: http.StatusBadRequest, Code: "invalid_argument", Message: "sandboxID is required"}
	}
	base := strings.TrimRight(firstNonEmpty(c.BaseURL,
		envFirst("GPURUN_BOXD_URL"), DefaultBaseURL), "/")
	target, err := url.Parse(base + path)
	if err != nil {
		return nil, "", &Error{Status: 0, Code: "invalid_url", Message: err.Error(), Err: err}
	}
	if sandboxID != "" {
		query := target.Query()
		query.Set("sandboxID", sandboxID)
		target.RawQuery = query.Encode()
	}
	return target, sandboxID, nil
}

func (c *Client) bearerToken() string {
	if c == nil {
		return ""
	}
	return firstNonEmpty(c.Token, c.APIKey, c.AccessToken,
		envFirst("GPURUN_API_KEY", "GPURUN_ACCESS_TOKEN", "NOVITA_API_KEY", "NOVITA_ACCESS_TOKEN"))
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{}
}

func successStatus(status int) bool {
	return status >= 200 && status < 300
}

func allowedStatus(status int, allowed ...int) bool {
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func decodeError(status int, data []byte) *Error {
	result := &Error{Status: status, Message: strings.TrimSpace(string(data))}
	if result.Message == "" {
		result.Message = http.StatusText(status)
	}
	if result.Message == "" {
		result.Message = "HTTP " + strconv.Itoa(status)
	}
	result.Code = codeForStatus(status)
	var envelope struct {
		Message string `json:"message"`
		Error   any    `json:"error"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil {
		if envelope.Message != "" {
			result.Message = envelope.Message
		} else if text, ok := envelope.Error.(string); ok && text != "" {
			result.Message = text
		} else if nested, ok := envelope.Error.(map[string]any); ok {
			if message, ok := nested["message"].(string); ok && message != "" {
				result.Message = message
			}
		} else if envelope.Detail != "" {
			result.Message = envelope.Detail
		}
	}
	return result
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusUnprocessableEntity:
		return "unprocessable_entity"
	default:
		if status >= 500 {
			return "internal_error"
		}
		return "http_error"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if trimmed := strings.TrimSpace(os.Getenv(key)); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		result[key] = value
	}
	return result
}

func asError(err error) *Error {
	if err == nil {
		return nil
	}
	var result *Error
	if errors.As(err, &result) {
		return result
	}
	return &Error{Status: 0, Code: "client_error", Message: err.Error(), Err: err}
}
