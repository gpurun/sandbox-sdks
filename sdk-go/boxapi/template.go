package boxapi

import (
	"net/http"
	"net/url"
)

func (c *Client) ListTemplates() ([]CompatibleTemplate, error) {
	var out []CompatibleTemplate
	if _, _, err := c.request(http.MethodGet, "/templates", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetTemplate(id string) (*TemplateWithBuilds, error) {
	var out TemplateWithBuilds
	if _, _, err := c.request(http.MethodGet, "/templates/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteTemplate(id string) error {
	_, _, err := c.request(http.MethodDelete, "/templates/"+url.PathEscape(id), nil, nil, nil)
	return err
}

func (c *Client) GetTemplateBuildStatus(templateID, buildID string) (*TemplateBuildInfo, error) {
	var out TemplateBuildInfo
	path := "/templates/" + url.PathEscape(templateID) + "/builds/" + url.PathEscape(buildID) + "/status"
	if _, _, err := c.request(http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StartTemplateBuildV2(templateID, buildID string, req any) error {
	path := "/v2/templates/" + url.PathEscape(templateID) + "/builds/" + url.PathEscape(buildID)
	_, _, err := c.request(http.MethodPost, path, req, nil, nil, http.StatusAccepted)
	return err
}

func (c *Client) CreateTemplateV3(req any) (*TemplateV3, error) {
	var out TemplateV3
	if _, _, err := c.request(http.MethodPost, "/v3/templates", req, nil, &out, http.StatusAccepted); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ConvertTemplate(req any) (*ConvertResult, error) {
	var out ConvertResult
	if _, _, err := c.request(http.MethodPost, "/v1/templates/convert", req, nil, &out, http.StatusCreated); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateImage(req CreateImageRequest) (*ImageInfo, error) {
	var out ImageInfo
	if _, _, err := c.request(http.MethodPost, "/v1/images", req, nil, &out, http.StatusCreated); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListImages() (*ImageList, error) {
	var out ImageList
	if _, _, err := c.request(http.MethodGet, "/v1/images", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetImage(id string) (*ImageInfo, error) {
	var out ImageInfo
	if _, _, err := c.request(http.MethodGet, "/v1/images/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteImage(id string) error {
	_, _, err := c.request(http.MethodDelete, "/v1/images/"+url.PathEscape(id), nil, nil, nil)
	return err
}

func (c *Client) ListRuntimes() (*RuntimeList, error) {
	var out RuntimeList
	if _, _, err := c.request(http.MethodGet, "/v1/runtimes", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetRuntime(runtimeType string) (*RuntimeSummary, error) {
	var out RuntimeSummary
	if _, _, err := c.request(http.MethodGet, "/v1/runtimes/"+url.PathEscape(runtimeType), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetRuntimeCapabilities(runtimeType string) (*RuntimeCapabilities, error) {
	var out RuntimeCapabilities
	path := "/v1/runtimes/" + url.PathEscape(runtimeType) + "/capabilities"
	if _, _, err := c.request(http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
