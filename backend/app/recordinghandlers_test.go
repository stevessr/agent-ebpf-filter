package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/app/recording"
)

func TestHandleSaveBrowserRecordingRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &spaceReader{remaining: recording.BrowserRecordingRequestMaxBytes + 1}
	req := httptest.NewRequest(http.MethodPost, "/events/recording/browser/save", reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	handleSaveBrowserRecording(ctx)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleReplayEventRecordingStopsOnCanceledRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/events/recording/replay", strings.NewReader(`{"path":"events.jsonl"}`)).WithContext(requestContext)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = req
	handleReplayEventRecording(ginContext)
	if recorder.Body.Len() != 0 {
		t.Fatalf("canceled replay wrote response body %q", recorder.Body.String())
	}
}

type spaceReader struct {
	remaining int64
}

func (reader *spaceReader) Read(buffer []byte) (int, error) {
	if reader.remaining <= 0 {
		return 0, errors.New("unexpected end")
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	for index := range buffer {
		buffer[index] = ' '
	}
	reader.remaining -= int64(len(buffer))
	return len(buffer), nil
}
