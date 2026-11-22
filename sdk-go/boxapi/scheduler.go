package boxapi

import (
	"net/http"
	"net/url"
)

func (c *Client) ListStrategies() (*StrategyList, error) {
	var out StrategyList
	if _, _, err := c.request(http.MethodGet, "/v1/scheduler/strategies", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SetStrategy(req any) (*Strategy, error) {
	var out Strategy
	if _, _, err := c.request(http.MethodPost, "/v1/scheduler/strategies", req, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetStrategy(id string) (*Strategy, error) {
	var out Strategy
	if _, _, err := c.request(http.MethodGet, "/v1/scheduler/strategies/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteStrategy(id string) (*DeleteStrategyResult, error) {
	var out DeleteStrategyResult
	if _, _, err := c.request(http.MethodDelete, "/v1/scheduler/strategies/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetDefaultStrategy() (*Strategy, error) {
	var out Strategy
	if _, _, err := c.request(http.MethodGet, "/v1/scheduler/strategies/default", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SetDefaultStrategy(id string) (*DefaultStrategyResult, error) {
	var out DefaultStrategyResult
	body := map[string]string{"id": id}
	if _, _, err := c.request(http.MethodPut, "/v1/scheduler/strategies/default", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetQuotas() (*QuotaList, error) {
	var out QuotaList
	if _, _, err := c.request(http.MethodGet, "/v1/scheduler/quotas", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateQuota(req any) (*Quota, error) {
	var out Quota
	if _, _, err := c.request(http.MethodPut, "/v1/scheduler/quotas", req, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Decide(req DecideRequest) (*Decision, error) {
	var out Decision
	if _, _, err := c.request(http.MethodPost, "/v1/scheduler/decide", req, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
