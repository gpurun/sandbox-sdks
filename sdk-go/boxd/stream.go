package boxd

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type ProcessStream struct {
	response *http.Response
	onEvent  func(ProcessEvent)
	done     chan struct{}
	result   socketResult
	once     sync.Once
	mu       sync.Mutex
	closed   bool
}

func newProcessStream(response *http.Response, onEvent func(ProcessEvent)) *ProcessStream {
	stream := &ProcessStream{
		response: response,
		onEvent:  onEvent,
		done:     make(chan struct{}),
	}
	go stream.run()
	return stream
}

func (s *ProcessStream) Wait() (ProcessExit, error) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result.exit, s.result.err
}

func (s *ProcessStream) Close() error {
	s.mu.Lock()
	s.closed = true
	response := s.response
	s.mu.Unlock()
	err := response.Body.Close()
	s.finish(ProcessExit{}, &Error{Status: 499, Code: "closed", Message: "process stream closed by client"})
	return err
}

func (s *ProcessStream) run() {
	for {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return
		}
		header := make([]byte, 5)
		if _, err := io.ReadFull(s.response.Body, header); err != nil {
			s.finish(ProcessExit{}, &Error{Status: 502, Code: "invalid_response", Message: err.Error(), Err: err})
			return
		}
		flags := header[0]
		length := int64(binary.BigEndian.Uint32(header[1:5]))
		if length > 64<<20 {
			s.finish(ProcessExit{}, &Error{Status: 502, Code: "invalid_response", Message: "Connect frame too large"})
			return
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(s.response.Body, payload); err != nil {
			s.finish(ProcessExit{}, &Error{Status: 502, Code: "invalid_response", Message: err.Error(), Err: err})
			return
		}

		var event ProcessEvent
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &event); err != nil {
				s.finish(ProcessExit{}, &Error{Status: 502, Code: "invalid_response", Message: "invalid Connect event frame", Err: err})
				return
			}
			if s.onEvent != nil {
				s.onEvent(event)
			}
		}
		if flags&0x02 != 0 || event.Event.End != nil {
			exit := ProcessExit{Exited: true}
			if event.Event.End != nil {
				parsed, err := parseProcessExitJSON(*event.Event.End)
				if err != nil {
					s.finish(ProcessExit{}, &Error{Status: 502, Code: "invalid_response", Message: err.Error(), Err: err})
					return
				}
				exit = parsed
			}
			s.finish(exit, nil)
			return
		}
	}
}

func (s *ProcessStream) finish(exit ProcessExit, err error) {
	s.once.Do(func() {
		s.mu.Lock()
		s.result = socketResult{exit: exit, err: err}
		close(s.done)
		s.mu.Unlock()
	})
}

func parseProcessExit(raw json.RawMessage) (ProcessExit, error) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || string(raw) == "null" {
		return ProcessExit{Exited: true}, nil
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ProcessExit{}, err
	}
	exit := ProcessExit{Exited: true}
	if value, ok := firstJSONNumber(fields, "exitCode", "exit_code", "code"); ok {
		exit.ExitCode = value
	}
	if value, ok := fields["exited"]; ok {
		var flag bool
		if err := json.Unmarshal(value, &flag); err == nil {
			exit.Exited = flag
		} else {
			var text string
			if err := json.Unmarshal(value, &text); err == nil {
				if parsed, err := strconv.ParseBool(strings.TrimSpace(text)); err == nil {
					exit.Exited = parsed
				}
			}
		}
	}
	if value, ok := firstJSONString(fields, "status"); ok {
		exit.Status = value
	}
	if value, ok := firstJSONString(fields, "error", "message"); ok {
		exit.Error = value
	}
	return exit, nil
}

func parseProcessExitJSON(exit ProcessExit) (ProcessExit, error) {
	data, err := json.Marshal(exit)
	if err != nil {
		return ProcessExit{}, err
	}
	return parseProcessExit(data)
}
