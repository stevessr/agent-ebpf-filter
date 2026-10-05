package domainforwardproxy

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

type sseRewriteBody struct {
	source      io.ReadCloser
	reader      *bufio.Reader
	kernel      *RewriteKernel
	host        string
	path        string
	contentType string
	mapping     *modelRewrite
	pending     []byte
	terminalErr error
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
		line, err := r.reader.ReadBytes('\n')
		if len(line) == 0 {
			return 0, err
		}
		if err != nil {
			r.terminalErr = err
		}
		r.pending = r.rewriteLine(line)
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *sseRewriteBody) rewriteLine(line []byte) []byte {
	if r.kernel == nil || !r.kernel.enabled || len(line) == 0 {
		return line
	}
	if int64(len(line)) > r.kernel.MaxBodyBytes() {
		return line
	}
	trimmed := bytes.TrimSpace(line)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		payload := bytes.TrimSpace(trimmed[len("data:"):])
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			return line
		}
		rewritten, changed := r.kernel.RewriteResponse(
			r.host,
			r.path,
			r.contentType,
			payload,
			r.mapping,
		)
		if !changed {
			return line
		}
		suffix := []byte{}
		if bytes.HasSuffix(line, []byte("\r\n")) {
			suffix = []byte("\r\n")
		} else if bytes.HasSuffix(line, []byte("\n")) {
			suffix = []byte("\n")
		}
		out := make([]byte, 0, len(rewritten)+len(suffix)+6)
		out = append(out, "data: "...)
		out = append(out, rewritten...)
		out = append(out, suffix...)
		return out
	}
	if len(r.kernel.bodyRules) == 0 || !strings.Contains(strings.ToLower(r.contentType), "text/event-stream") {
		return line
	}
	rewritten, changed := r.kernel.applyLiteralRules(
		"response",
		r.host,
		r.path,
		r.contentType,
		line,
	)
	if !changed {
		return line
	}
	return rewritten
}
