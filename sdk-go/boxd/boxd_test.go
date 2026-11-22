package boxd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/websocket"
)

func TestClientSandboxHeaderAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("sandboxID") != "sbx-1" {
			t.Errorf("sandboxID query = %q", r.URL.Query().Get("sandboxID"))
		}
		if r.Header.Get(SandboxIDHeader) != "sbx-1" {
			t.Errorf("sandbox header = %q", r.Header.Get(SandboxIDHeader))
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"status":"ok","service":"boxd"}`))
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL, SandboxID: "sbx-1", Token: "token"})
	status, err := client.Health()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "ok" || status.Service != "boxd" {
		t.Fatalf("status = %#v", status)
	}
}

func TestHealthWithoutSandboxID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want empty", r.URL.RawQuery)
		}
		if r.Header.Get(SandboxIDHeader) != "" {
			t.Errorf("sandbox header = %q, want empty", r.Header.Get(SandboxIDHeader))
		}
		_, _ = w.Write([]byte(`{"status":"ok","service":"boxproxy"}`))
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL})
	status, err := client.Health()
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "ok" || status.Service != "boxproxy" {
		t.Fatalf("status = %#v", status)
	}
	_, err = client.Exec([]string{"true"}, nil)
	var boxdErr *Error
	if !errors.As(err, &boxdErr) || boxdErr.Message != "sandboxID is required" {
		t.Fatalf("exec error = %v, want sandboxID is required", err)
	}
}

func TestExecParses200And422(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		exitCode int
		stdout   string
		stderr   string
	}{
		{"ok", http.StatusOK, `{"exitCode":0,"stdout":"hello","stderr":""}`, 0, "hello", ""},
		{"nonzero", http.StatusUnprocessableEntity, `{"exitCode":7,"stdout":"out","stderr":"err"}`, 7, "out", "err"},
		{"nonzero_default", http.StatusUnprocessableEntity, `{}`, 1, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if r.URL.Query().Get("sandboxID") != "sbx" || r.Header.Get(SandboxIDHeader) != "sbx" {
					t.Errorf("missing sandbox identifiers: %v %v", r.URL, r.Header)
				}
				cmd, ok := body["cmd"].([]any)
				if !ok || len(cmd) != 2 || cmd[0] != "echo" {
					t.Errorf("cmd = %#v", body["cmd"])
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := New(ClientOptions{BaseURL: server.URL, SandboxID: "sbx"})
			result, err := client.Exec([]string{"echo", "hi"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.ExitCode != tc.exitCode || result.Stdout != tc.stdout || result.Stderr != tc.stderr {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestExec400IsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad cmd"}`))
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL, SandboxID: "sbx"})
	_, err := client.Exec([]string{"echo"}, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T", err)
	}
	if apiErr.Status != http.StatusBadRequest || apiErr.Message != "bad cmd" {
		t.Fatalf("error = %#v", apiErr)
	}
}

func writeConnectFrame(w http.ResponseWriter, flags byte, payload string) {
	header := []byte{flags, 0, 0, 0, byte(len(payload))}
	_, _ = w.Write(header)
	_, _ = w.Write([]byte(payload))
}

func TestConnectStreamFrames(t *testing.T) {
	var events []ProcessEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/process.Process/Start" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != ConnectContentType {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.URL.Query().Get("sandboxID") != "sbx" || r.Header.Get(SandboxIDHeader) != "sbx" {
			t.Errorf("missing sandbox identifiers: %v %v", r.URL, r.Header)
		}
		writeConnectFrame(w, 0, `{"event":{"start":{"pid":7}}}`)
		writeConnectFrame(w, 0, `{"event":{"data":{"stdout":"aGVsbG8="}}}`)
		writeConnectFrame(w, 0, `{"event":{"keepalive":{}}}`)
		writeConnectFrame(w, 2, `{"event":{"end":{"exitCode":3,"exited":false,"status":"failed"}}}`)
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL, SandboxID: "sbx"})
	stream, err := client.StartProcessStream(StartProcessStreamRequest{
		Process: ProcessStreamConfig{Cmd: "echo"},
	}, func(event ProcessEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	exit, err := stream.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if exit.ExitCode != 3 || exit.Exited || exit.Status != "failed" {
		t.Fatalf("exit = %#v", exit)
	}
	if len(events) != 4 {
		t.Fatalf("events = %d", len(events))
	}
	if events[1].Event.Data == nil || events[1].Event.Data.Stdout != "aGVsbG8=" {
		t.Fatalf("data event = %#v", events[1])
	}
	if events[2].Event.Keepalive == nil {
		t.Fatalf("keepalive event = %#v", events[2])
	}
}

func TestConnectProcessWebSocket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/processes/proc-1/connect" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("sandboxID") != "sbx" || r.Header.Get(SandboxIDHeader) != "sbx" {
			t.Errorf("missing sandbox identifiers: %v %v", r.URL, r.Header)
		}
		server := websocket.Server{Handler: func(ws *websocket.Conn) {
			_ = websocket.Message.Send(ws, `{"type":"start","process":{"pid":7}}`)
			_ = websocket.Message.Send(ws, []byte("hello"))
			_ = websocket.Message.Send(ws, `{"type":"end","exit":{"exitCode":0,"exited":true,"status":"completed"}}`)
		}}
		server.ServeHTTP(w, r)
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL, SandboxID: "sbx"})
	var output []byte
	var ended ProcessExit
	var events []ProcessWSEvent
	socket, err := client.ConnectProcess("proc-1",
		func(chunk []byte) { output = append(output, chunk...) },
		func(exit ProcessExit) { ended = exit },
		WithProcessEvent(func(event ProcessWSEvent) { events = append(events, event) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	exit, err := socket.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "hello" || exit.ExitCode != 0 || exit.Status != "completed" || ended.Status != "completed" {
		t.Fatalf("output=%q exit=%#v ended=%#v", output, exit, ended)
	}
	if len(events) != 2 || events[0].Type != "start" || events[0].Process == nil || events[0].Process.PID != 7 {
		t.Fatalf("events = %#v", events)
	}
	if events[1].Type != "end" || events[1].Exit == nil || events[1].Exit.Status != "completed" {
		t.Fatalf("events = %#v", events)
	}
}

func TestSendInputBase64Encodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/process.Process/SendInput" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != ConnectContentType {
			t.Errorf("Content-Type = %q", contentType)
		}
		var body struct {
			Input struct {
				Stdin string `json:"stdin"`
				Pty   string `json:"pty"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Input.Stdin != "aGk=" || body.Input.Pty != "Aw==" {
			t.Errorf("input = %#v, want base64 hi/0x03", body.Input)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL, SandboxID: "sbx"})
	err := client.SendInput(SendInputRequest{
		Process: ProcessSelector{PID: 7},
		Input:   SendInputPayload{Stdin: "hi", Pty: "\x03"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConnectProcessWebSocketBinaryEventFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsServer := websocket.Server{Handler: func(ws *websocket.Conn) {
			_ = websocket.Message.Send(ws, []byte(`{"type":"start","process":{"pid":7}}`))
			_ = websocket.Message.Send(ws, []byte("hello"))
			_ = websocket.Message.Send(ws, []byte(`{"type":"end","exit":{"exitCode":3,"exited":true,"status":"completed"}}`))
		}}
		wsServer.ServeHTTP(w, r)
	}))
	defer server.Close()

	client := New(ClientOptions{BaseURL: server.URL, SandboxID: "sbx"})
	var output []byte
	var events []ProcessWSEvent
	socket, err := client.ConnectProcess("proc-1",
		func(chunk []byte) { output = append(output, chunk...) },
		nil,
		WithProcessEvent(func(event ProcessWSEvent) { events = append(events, event) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	exit, err := socket.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "hello" || exit.ExitCode != 3 || exit.Status != "completed" {
		t.Fatalf("output=%q exit=%#v", output, exit)
	}
	if len(events) != 2 || events[0].Type != "start" || events[0].Process == nil || events[0].Process.PID != 7 {
		t.Fatalf("events = %#v", events)
	}
	if events[1].Type != "end" || events[1].Exit == nil || events[1].Exit.ExitCode != 3 {
		t.Fatalf("events = %#v", events)
	}
}

func TestProcessDataEventDecodesBase64(t *testing.T) {
	event := ProcessDataEvent{Stdout: "aGVsbG8="}
	out, err := event.StdoutBytes()
	if err != nil || string(out) != "hello" {
		t.Fatalf("StdoutBytes = %q, %v", out, err)
	}
	pty, err := event.PtyBytes()
	if err != nil || pty != nil {
		t.Fatalf("PtyBytes = %v, %v", pty, err)
	}
	if _, err := (ProcessDataEvent{Stdout: "!!!"}).StdoutBytes(); err == nil {
		t.Fatal("expected base64 decode error")
	}
}
