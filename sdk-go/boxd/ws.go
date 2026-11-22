package boxd

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

var errWebSocketClosed = errors.New("websocket closed")

type websocketFrame struct {
	fin     bool
	opcode  byte
	payload []byte
}

type websocketConn struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

type socketResult struct {
	exit ProcessExit
	err  error
}

type ProcessSocket struct {
	ws       *websocketConn
	onOutput func([]byte)
	onEnd    func(ProcessExit)
	done     chan struct{}
	result   socketResult
	once     sync.Once
	mu       sync.Mutex
	closed   bool
}

func (c *Client) ConnectProcess(id string, onOutput func([]byte), onEnd func(ProcessExit)) (*ProcessSocket, error) {
	target, sandboxID, err := c.target("/processes/" + url.PathEscape(id) + "/connect")
	if err != nil {
		return nil, err
	}
	switch target.Scheme {
	case "http":
		target.Scheme = "ws"
	case "https":
		target.Scheme = "wss"
	}

	headers := map[string]string{}
	for key, value := range c.Headers {
		headers[key] = value
	}
	headers[SandboxIDHeader] = sandboxID
	if token := c.bearerToken(); token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	ws, err := dialWebSocket(context.Background(), target, headers, c.Timeout)
	if err != nil {
		return nil, err
	}
	socket := &ProcessSocket{
		ws:       ws,
		onOutput: onOutput,
		onEnd:    onEnd,
		done:     make(chan struct{}),
	}
	go socket.run()
	return socket, nil
}

func (s *ProcessSocket) Write(data []byte) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return &Error{Status: 0, Code: "closed", Message: "process socket is closed"}
	}
	ws := s.ws
	s.mu.Unlock()
	return ws.writeFrame(0x02, data)
}

func (s *ProcessSocket) Close() error {
	s.mu.Lock()
	s.closed = true
	ws := s.ws
	s.mu.Unlock()
	err := ws.close()
	s.finish(ProcessExit{}, &Error{Status: 499, Code: "closed", Message: "process socket closed by client"})
	return err
}

func (s *ProcessSocket) Wait() (ProcessExit, error) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result.exit, s.result.err
}

func (s *ProcessSocket) run() {
	for {
		opcode, payload, err := s.ws.readMessage()
		if err != nil {
			status := 502
			if errors.Is(err, errWebSocketClosed) {
				status = 502
			}
			s.finish(ProcessExit{}, &Error{Status: status, Code: "websocket_error", Message: err.Error(), Err: err})
			return
		}
		switch opcode {
		case 0x01:
			if s.handleEventPayload(payload) {
				return
			}
		case 0x02:
			// boxproxy 会把 boxd 的 TEXT 事件帧改写成 BINARY，
			// 所以 BINARY 帧要先探测是否为 JSON 事件，否则才是 stdout 数据。
			if isWSEventPayload(payload) {
				if s.handleEventPayload(payload) {
					return
				}
				continue
			}
			if s.onOutput != nil {
				s.onOutput(payload)
			}
		}
	}
}

func (s *ProcessSocket) handleEventPayload(payload []byte) bool {
	var wire struct {
		Type string          `json:"type"`
		Exit json.RawMessage `json:"exit"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		s.finish(ProcessExit{}, &Error{Status: 502, Code: "invalid_response", Message: "invalid WebSocket JSON frame", Err: err})
		return true
	}
	if wire.Type != "end" {
		return false
	}
	exit, err := parseProcessExit(wire.Exit)
	s.finish(exit, err)
	return true
}

func isWSEventPayload(payload []byte) bool {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil {
		return false
	}
	switch probe.Type {
	case "start", "data", "end", "keepalive":
		return true
	}
	return false
}

func (s *ProcessSocket) finish(exit ProcessExit, err error) {
	s.once.Do(func() {
		s.mu.Lock()
		s.result = socketResult{exit: exit, err: err}
		callback := s.onEnd
		s.mu.Unlock()
		if err == nil && callback != nil {
			callback(exit)
		}
		close(s.done)
	})
}

func dialWebSocket(ctx context.Context, target *url.URL, headers map[string]string, timeout time.Duration) (*websocketConn, error) {
	host := target.Host
	if target.Port() == "" {
		port := "80"
		if target.Scheme == "wss" {
			port = "443"
		}
		host = net.JoinHostPort(target.Hostname(), port)
	}
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, &Error{Status: 0, Code: "network_error", Message: err.Error(), Err: err}
	}
	if target.Scheme == "wss" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: target.Hostname()})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, &Error{Status: 0, Code: "network_error", Message: err.Error(), Err: err}
		}
		conn = tlsConn
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = conn.Close()
		return nil, &Error{Status: 0, Code: "network_error", Message: err.Error(), Err: err}
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := target.RequestURI()
	if path == "" {
		path = "/"
	}
	var handshake strings.Builder
	handshake.WriteString("GET " + path + " HTTP/1.1\r\n")
	handshake.WriteString("Host: " + target.Host + "\r\n")
	handshake.WriteString("Upgrade: websocket\r\n")
	handshake.WriteString("Connection: Upgrade\r\n")
	handshake.WriteString("Sec-WebSocket-Key: " + key + "\r\n")
	handshake.WriteString("Sec-WebSocket-Version: 13\r\n")
	for name, value := range headers {
		handshake.WriteString(name + ": " + value + "\r\n")
	}
	handshake.WriteString("\r\n")
	if _, err := io.WriteString(conn, handshake.String()); err != nil {
		_ = conn.Close()
		return nil, &Error{Status: 0, Code: "network_error", Message: err.Error(), Err: err}
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, &Error{Status: 502, Code: "handshake_failed", Message: err.Error(), Err: err}
	}
	parts := strings.SplitN(strings.TrimSpace(statusLine), " ", 3)
	status := 0
	if len(parts) > 1 {
		status, _ = strconv.Atoi(parts[1])
	}
	if status != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return nil, &Error{Status: status, Code: "handshake_failed", Message: strings.TrimSpace(statusLine)}
	}

	accept := ""
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, &Error{Status: 502, Code: "handshake_failed", Message: err.Error(), Err: err}
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Sec-WebSocket-Accept") {
			accept = strings.TrimSpace(value)
		}
	}
	digest := sha1.Sum([]byte(key + websocketGUID))
	expected := base64.StdEncoding.EncodeToString(digest[:])
	if accept != expected {
		_ = conn.Close()
		return nil, &Error{Status: 502, Code: "handshake_failed", Message: "WebSocket handshake accept mismatch"}
	}
	return &websocketConn{conn: conn, reader: reader}, nil
}

func (ws *websocketConn) writeFrame(opcode byte, payload []byte) error {
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	header := make([]byte, 0, 14)
	header = append(header, 0x80|opcode)
	switch {
	case len(payload) < 126:
		header = append(header, 0x80|byte(len(payload)))
	case len(payload) <= 0xffff:
		header = append(header, 0x80|126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(len(payload)))
	default:
		header = append(header, 0x80|127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(len(payload)))
	}
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	frame := append(header, mask...)
	frame = append(frame, masked...)
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	_, err := ws.conn.Write(frame)
	return err
}

func (ws *websocketConn) readMessage() (byte, []byte, error) {
	var fragmentOpcode byte
	var fragments []byte
	for {
		frame, err := ws.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch frame.opcode {
		case 0x09:
			if err := ws.writeFrame(0x0a, frame.payload); err != nil {
				return 0, nil, err
			}
		case 0x0a:
			continue
		case 0x08:
			return 0, nil, errWebSocketClosed
		case 0x01, 0x02:
			if frame.fin {
				return frame.opcode, frame.payload, nil
			}
			fragmentOpcode = frame.opcode
			fragments = append([]byte(nil), frame.payload...)
		case 0x00:
			fragments = append(fragments, frame.payload...)
			if frame.fin {
				return fragmentOpcode, fragments, nil
			}
		default:
			return 0, nil, errors.New("unsupported WebSocket opcode")
		}
	}
}

func (ws *websocketConn) readFrame() (websocketFrame, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(ws.reader, header); err != nil {
		return websocketFrame{}, err
	}
	frame := websocketFrame{
		fin:    header[0]&0x80 != 0,
		opcode: header[0] & 0x0f,
	}
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(ws.reader, extended); err != nil {
			return websocketFrame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(extended))
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(ws.reader, extended); err != nil {
			return websocketFrame{}, err
		}
		length = binary.BigEndian.Uint64(extended)
	}
	if length > 64<<20 {
		return websocketFrame{}, errors.New("WebSocket payload too large")
	}
	var mask []byte
	if masked {
		mask = make([]byte, 4)
		if _, err := io.ReadFull(ws.reader, mask); err != nil {
			return websocketFrame{}, err
		}
	}
	frame.payload = make([]byte, length)
	if _, err := io.ReadFull(ws.reader, frame.payload); err != nil {
		return websocketFrame{}, err
	}
	if masked {
		for i := range frame.payload {
			frame.payload[i] ^= mask[i%4]
		}
	}
	return frame, nil
}

func (ws *websocketConn) close() error {
	_ = ws.writeFrame(0x08, nil)
	return ws.conn.Close()
}
