package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizeCameraDeviceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: "/dev/video0"},
		{name: "trimmed", input: "  /dev/video12  ", want: "/dev/video12"},
		{name: "relative", input: "video0", wantErr: true},
		{name: "non numeric suffix", input: "/dev/video-test", wantErr: true},
		{name: "path traversal", input: "/dev/video0/../../etc/passwd", wantErr: true},
		{name: "arbitrary device", input: "/dev/mem", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeCameraDeviceName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeCameraDeviceName(%q) = %q, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeCameraDeviceName(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeCameraDeviceName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestHandleCameraSnapshotHandlesMissingStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDeps := Deps
	t.Cleanup(func() { Deps = previousDeps })
	Deps.GetCameraStream = func(string) *CameraStream { return nil }

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/camera/snapshot?device=/dev/video0", nil)
	HandleCameraSnapshot(ctx)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
