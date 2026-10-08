package domainforwardproxy

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

type sseRewriteBody struct {
	source              io.ReadCloser
	reader              *bufio.Reader
	kernel              *RewriteKernel
	host                string
	path                string
	contentType         string
	mapping             *modelRewrite
	pending             []byte
	terminalErr         error
	passthroughEvent    bool
	passthroughLineOpen bool
}

func newSSERewriteBody(
	source io.ReadCloser,
	kernel *RewriteKernel,
	host, path, contentType string,
	mapping *modelRewrite,
) io.ReadCloser {
	return &sseRewriteBody{
		source:      source,
		reader:      bufio.NewReaderSize(source, 32<<10),
		kernel:      kernel,
		host:        host,
		path:        path,
		contentType: contentType,
		mapping:     mapping,
	}
}

func (r *sseRewriteBody) Close() error {
	return r.source.Close()
}

func (r *sseRewriteBody) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.terminalErr != nil {
			err := r.terminalErr
			r.terminalErr = nil
			return 0, err
		}
		if r.passthroughEvent {
			chunk, err := r.readPassthroughChunk()
			if len(chunk) == 0 {
				return 0, err
			}
			if err != nil {
				r.terminalErr = err
			}
			r.pending = chunk
			continue
		}
		event, err, overLimit := r.readEvent()
		if len(event) == 0 {
			return 0, err
		}
		if err != nil {
			r.terminalErr = err
		}
		if overLimit {
			r.pending = event
			continue
		}
		r.pending = r.rewriteEvent(event)
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *sseRewriteBody) readEvent() ([]byte, error, bool) {
	limit := defaultRewriteBodyBytes
	if r.kernel != nil {
		limit = r.kernel.MaxBodyBytes()
	}
	event := make([]byte, 0, 1024)
	lineLen := 0
	for {
		fragment, err := r.reader.ReadSlice('\n')
		if len(fragment) > 0 {
			event = append(event, fragment...)
			lineLen += len(fragment)
		}
		if int64(len(event)) > limit {
			if errors.Is(err, bufio.ErrBufferFull) {
				r.passthroughEvent = true
				r.passthroughLineOpen = true
			} else if err == nil {
				r.passthroughEvent = !isSSEBlankLineLength(event, lineLen)
				r.passthroughLineOpen = false
			}
			return event, normalizeSSEReadError(err), true
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			return event, err, false
		}
		if isSSEBlankLineLength(event, lineLen) {
			return event, nil, false
		}
		lineLen = 0
	}
}

func (r *sseRewriteBody) readPassthroughChunk() ([]byte, error) {
	fragment, err := r.reader.ReadSlice('\n')
	if len(fragment) == 0 {
		return nil, err
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		r.passthroughLineOpen = true
		return append([]byte(nil), fragment...), nil
	}
	if !r.passthroughLineOpen && isSSEBlankLine(fragment) {
		r.passthroughEvent = false
	}
	r.passthroughLineOpen = false
	return append([]byte(nil), fragment...), err
}

func normalizeSSEReadError(err error) error {
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil
	}
	return err
}

func isSSEBlankLineLength(event []byte, lineLen int) bool {
	if lineLen <= 0 || lineLen > len(event) {
		return false
	}
	return isSSEBlankLine(event[len(event)-lineLen:])
}

func isSSEBlankLine(line []byte) bool {
	return bytes.Equal(line, []byte("\n")) || bytes.Equal(line, []byte("\r\n"))
}

func (r *sseRewriteBody) rewriteEvent(event []byte) []byte {
	if r.kernel == nil || !r.kernel.enabled || len(event) == 0 {
		return event
	}

	lines := bytes.SplitAfter(event, []byte("\n"))
	dataValues := make([][]byte, 0, 1)
	firstData := -1
	for i, line := range lines {
		value, ok := sseDataValue(line)
		if !ok {
			continue
		}
		if firstData < 0 {
			firstData = i
		}
		dataValues = append(dataValues, value)
	}
	if firstData < 0 || len(dataValues) == 0 {
		return event
	}

	payload := bytes.Join(dataValues, []byte("\n"))
	if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
		return event
	}
	payloadContentType := "text/plain"
	if looksLikeJSONPayload(payload) {
		payloadContentType = "application/json"
	}
	rewritten, changed := r.kernel.rewriteResponseWithContentTypes(
		r.host,
		r.path,
		payloadContentType,
		r.contentType,
		payload,
		r.mapping,
	)
	if !changed {
		return event
	}

	lineEnding := sseLineEnding(lines[firstData])
	if len(lineEnding) == 0 {
		lineEnding = []byte("\n")
	}
	replacement := renderSSEDataLines(rewritten, lineEnding)

	out := make([]byte, 0, len(event)+len(replacement))
	inserted := false
	for i, line := range lines {
		if _, ok := sseDataValue(line); ok {
			if !inserted && i == firstData {
				out = append(out, replacement...)
				inserted = true
			}
			continue
		}
		out = append(out, line...)
	}
	return out
}

func sseDataValue(line []byte) ([]byte, bool) {
	content := trimSSELineEnding(line)
	if bytes.Equal(content, []byte("data")) {
		return nil, true
	}
	if !bytes.HasPrefix(content, []byte("data:")) {
		return nil, false
	}
	value := content[len("data:"):]
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return value, true
}

func trimSSELineEnding(line []byte) []byte {
	switch {
	case bytes.HasSuffix(line, []byte("\r\n")):
		return line[:len(line)-2]
	case bytes.HasSuffix(line, []byte("\n")):
		return line[:len(line)-1]
	default:
		return line
	}
}

func sseLineEnding(line []byte) []byte {
	switch {
	case bytes.HasSuffix(line, []byte("\r\n")):
		return []byte("\r\n")
	case bytes.HasSuffix(line, []byte("\n")):
		return []byte("\n")
	default:
		return nil
	}
}

func renderSSEDataLines(payload, lineEnding []byte) []byte {
	segments := bytes.Split(payload, []byte("\n"))
	out := make([]byte, 0, len(payload)+len(segments)*(6+len(lineEnding)))
	for _, segment := range segments {
		out = append(out, "data: "...)
		out = append(out, segment...)
		out = append(out, lineEnding...)
	}
	return out
}
