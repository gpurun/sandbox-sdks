package boxapi

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
		return fmt.Sprintf("boxapi: %s (status %d, code %s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("boxapi: %s (status %d)", e.Message, e.Status)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type Client struct {
	BaseURL     string
	Token       string
	APIKey      string
	AccessToken string
	Timeout     time.Duration
	HTTPClient  *http.Client
	Headers     map[string]string
}

type ClientOptions struct {
	BaseURL     string
	Token       string
	APIKey      string
	AccessToken string
	Timeout     time.Duration
	HTTPClient  *http.Client
	Headers     map[string]string
}

func NewClient(opts ClientOptions) (*Client, error) {
	return &Client{
		BaseURL:     firstNonEmpty(opts.BaseURL, envFirst("GPURUN_BOXAPI_URL"), DefaultBaseURL),
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

func (c *Client) request(method, path string, body any, query url.Values, out any, allowed ...int) ([]byte, http.Header, error) {
	return c.requestWithContext(context.Background(), method, path, body, query, out, allowed...)
}

func (c *Client) requestWithContext(ctx context.Context, method, path string, body any, query url.Values, out any, allowed ...int) ([]byte, http.Header, error) {
	if c == nil {
		return nil, nil, &Error{Status: 0, Code: "invalid_client", Message: "nil boxapi client"}
	}

	target, err := url.Parse(strings.TrimRight(c.baseURL(), "/") + path)
	if err != nil {
		return nil, nil, &Error{Status: 0, Code: "invalid_url", Message: err.Error(), Err: err}
	}
	if len(query) > 0 {
		existing := target.Query()
		for key, values := range query {
			for _, value := range values {
				existing.Add(key, value)
			}
		}
		target.RawQuery = existing.Encode()
	}

	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, nil, &Error{Status: 0, Code: "invalid_request", Message: err.Error(), Err: err}
		}
	}

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, nil, &Error{Status: 0, Code: "invalid_request", Message: err.Error(), Err: err}
	}
	req.Header.Set("Accept", "application/json")
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
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

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, &Error{Status: 0, Code: "network_error", Message: err.Error(), Err: err}
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, &Error{Status: res.StatusCode, Code: "network_error", Message: err.Error(), Err: err}
	}

	if !successStatus(res.StatusCode) && !allowedStatus(res.StatusCode, allowed...) {
		return nil, res.Header, decodeError(res.StatusCode, data)
	}
	if out != nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return data, res.Header, &Error{Status: res.StatusCode, Code: "invalid_response", Message: "invalid JSON response", Err: err}
		}
	}
	return data, res.Header, nil
}

func (c *Client) baseURL() string {
	if c == nil {
		return DefaultBaseURL
	}
	return firstNonEmpty(c.BaseURL, envFirst("GPURUN_BOXAPI_URL"), DefaultBaseURL)
}

func (c *Client) bearerToken() string {
	if c == nil {
		return ""
	}
	return firstNonEmpty(c.Token, c.APIKey, c.AccessToken,
		envFirst("GPURUN_API_KEY", "GPURUN_ACCESS_TOKEN", "NOVITA_API_KEY", "NOVITA_ACCESS_TOKEN"))
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
	result := &Error{
		Status:  status,
		Code:    codeForStatus(status),
		Message: strings.TrimSpace(string(data)),
	}
	if result.Message == "" {
		result.Message = http.StatusText(status)
	}
	if result.Message == "" {
		result.Message = "HTTP " + strconv.Itoa(status)
	}

	var wire struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &wire); err == nil {
		if wire.Code != "" {
			result.Code = wire.Code
		}
		if wire.Message != "" {
			result.Message = wire.Message
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
