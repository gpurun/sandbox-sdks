package boxapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAuthorizationAndCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sandboxes" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["templateID"] != "tpl" || body["image_id"] != "img" || body["runtime_type"] != "gvisor" {
			t.Errorf("body = %#v", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"alias":null,"clientID":"client","domain":null,"envdVersion":"1.2.3","sandboxID":"sbx","templateID":"tpl","trafficAccessToken":null}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := client.CreateSandbox(CreateSandboxRequest{
		TemplateID:  "tpl",
		ImageID:     "img",
		RuntimeType: "gvisor",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sandbox.SandboxID != "sbx" || sandbox.ClientID != "client" || sandbox.Alias != nil {
		t.Fatalf("sandbox = %#v", sandbox)
	}
}

func TestListSandboxesV2Pagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sandboxes" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		states := r.URL.Query()["state"]
		if len(states) != 2 || states[0] != "running" || states[1] != "paused" {
			t.Errorf("states = %#v", states)
		}
		if r.URL.Query().Get("nextToken") != "5" || r.URL.Query().Get("limit") != "50" {
			t.Errorf("query = %v", r.URL.Query())
		}
		w.Header().Set("X-Next-Token", "next-page")
		_, _ = w.Write([]byte(`[{"alias":null,"clientID":"c","cpuCount":2,"diskSizeMB":1024,"endAt":"","envdVersion":"1","memoryMB":512,"metadata":{},"sandboxID":"s","startedAt":"","state":"running","templateID":"t","volumeMounts":[]}]`))
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL})
	page, err := client.ListSandboxesV2(ListOptions{State: []string{"running", "paused"}, NextToken: 5, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextToken != "next-page" || len(page.Items) != 1 || page.Items[0].SandboxID != "s" {
		t.Fatalf("page = %#v", page)
	}
}

func TestListSandboxesV2AcceptsItemsEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"alias":null,"clientID":"c","cpuCount":2,"diskSizeMB":1024,"endAt":"","envdVersion":"1","memoryMB":512,"metadata":{},"sandboxID":"s2","startedAt":"","state":"running","templateID":"t","volumeMounts":[]}],"nextToken":"body-page"}`))
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL})
	page, err := client.ListSandboxesV2(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextToken != "body-page" || len(page.Items) != 1 || page.Items[0].SandboxID != "s2" {
		t.Fatalf("page = %#v", page)
	}
}

func TestErrorMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"custom_missing","message":"no such sandbox"}`))
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL})
	_, err := client.GetSandbox("missing")
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Code != "custom_missing" || apiErr.Message != "no such sandbox" {
		t.Fatalf("error = %#v", apiErr)
	}
}

func TestEnvironmentAuthPrecedence(t *testing.T) {
	t.Setenv("NOVITA_API_KEY", "old-key")
	t.Setenv("GPURUN_API_KEY", "new-key")
	t.Setenv("NOVITA_BASE_URL", "http://127.0.0.1:1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer new-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	t.Setenv("GPURUN_BOXAPI_URL", server.URL)

	client := New()
	status, err := client.Health()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "ok" {
		t.Fatalf("status = %#v", status)
	}
}

func TestDecideUsesPascalCase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["SandboxID"] != "sbx" || body["ProviderHint"] != "novita" {
			t.Errorf("body = %#v", body)
		}
		requirements, ok := body["Requirements"].(map[string]any)
		if !ok || requirements["MinCPU"] != float64(4) || requirements["NeedGPU"] != true {
			t.Errorf("requirements = %#v", body["Requirements"])
		}
		_, _ = w.Write([]byte(`{"provider":"novita","strategy":"s","reason":"ok","score":1}`))
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL})
	decision, err := client.Decide(DecideRequest{
		SandboxID:    "sbx",
		ProviderHint: "novita",
		Requirements: &DecisionRequirements{MinCPU: 4, NeedGPU: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Provider != "novita" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestTemplateAndV1SandboxRoutes(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodGet && r.URL.Path == "/templates" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL})
	calls := []struct {
		name   string
		method string
		path   string
		run    func() error
	}{
		{"ListTemplates", http.MethodGet, "/templates", func() error { _, err := client.ListTemplates(); return err }},
		{"GetTemplate", http.MethodGet, "/templates/tpl", func() error { _, err := client.GetTemplate("tpl"); return err }},
		{"DeleteTemplate", http.MethodDelete, "/templates/tpl", func() error { return client.DeleteTemplate("tpl") }},
		{"GetTemplateBuildStatus", http.MethodGet, "/templates/tpl/builds/b/status", func() error { _, err := client.GetTemplateBuildStatus("tpl", "b"); return err }},
		{"StartTemplateBuildV2", http.MethodPost, "/v2/templates/tpl/builds/b", func() error { return client.StartTemplateBuildV2("tpl", "b", map[string]any{}) }},
		{"ConvertTemplate", http.MethodPost, "/v1/templates/convert", func() error { _, err := client.ConvertTemplate(map[string]any{}); return err }},
		{"PoweroffSandbox", http.MethodPost, "/v1/sandboxes/sb/poweroff", func() error { _, err := client.PoweroffSandbox("sb"); return err }},
		{"PoweronSandbox", http.MethodPost, "/v1/sandboxes/sb/poweron", func() error { _, err := client.PoweronSandbox("sb"); return err }},
		{"RebootSandbox", http.MethodPost, "/v1/sandboxes/sb/reboot", func() error { _, err := client.RebootSandbox("sb"); return err }},
		{"GetBalloon", http.MethodGet, "/v1/sandboxes/sb/balloon", func() error { _, err := client.GetBalloon("sb"); return err }},
		{"UpdateBalloon", http.MethodPatch, "/v1/sandboxes/sb/balloon", func() error { _, err := client.UpdateBalloon("sb", 512); return err }},
		{"GetBalloonStats", http.MethodGet, "/v1/sandboxes/sb/balloon/statistics", func() error { _, err := client.GetBalloonStats("sb"); return err }},
		{"UpdateBalloonStats", http.MethodPatch, "/v1/sandboxes/sb/balloon/statistics", func() error { _, err := client.UpdateBalloonStats("sb", 15); return err }},
		{"GetBalloonHinting", http.MethodGet, "/v1/sandboxes/sb/balloon/hinting", func() error { _, err := client.GetBalloonHinting("sb"); return err }},
		{"StartBalloonHinting", http.MethodPost, "/v1/sandboxes/sb/balloon/hinting/start", func() error { _, err := client.StartBalloonHinting("sb", nil); return err }},
		{"StopBalloonHinting", http.MethodPost, "/v1/sandboxes/sb/balloon/hinting/stop", func() error { _, err := client.StopBalloonHinting("sb"); return err }},
		{"ConnectSandbox", http.MethodPost, "/sandboxes/sb/connect", func() error { _, err := client.ConnectSandbox("sb"); return err }},
		{"SetSandboxTimeout", http.MethodPost, "/sandboxes/sb/timeout", func() error { return client.SetSandboxTimeout("sb") }},
	}
	for _, call := range calls {
		seen = nil
		if err := call.run(); err != nil {
			t.Fatalf("%s: %v", call.name, err)
		}
		want := call.method + " " + call.path
		if len(seen) != 1 || seen[0] != want {
			t.Errorf("%s: got %v, want [%s]", call.name, seen, want)
		}
	}
}
