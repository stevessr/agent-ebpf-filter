package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-ebpf-filter/app/sandboxruntime"

	"github.com/gin-gonic/gin"
)

type fakeSandboxRuntimeOps struct{}

func (fakeSandboxRuntimeOps) Status() sandboxruntime.Status {
	return sandboxruntime.Status{Available: true, Platform: "linux", ProcRoot: "/proc"}
}

func (fakeSandboxRuntimeOps) DetectPID(pid int) (sandboxruntime.Detection, error) {
	return sandboxruntime.Detection{
		PID:         pid,
		Runtime:     "gVisor",
		Kind:        "gvisor",
		ContainerID: "sandbox-1",
	}, nil
}

func (fakeSandboxRuntimeOps) ListActive(limit int) ([]sandboxruntime.Detection, error) {
	return []sandboxruntime.Detection{{PID: 42, Runtime: "gVisor", Kind: "gvisor"}}, nil
}

func TestSandboxRuntimeHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := Deps.SandboxRuntime
	Deps.SandboxRuntime = fakeSandboxRuntimeOps{}
	t.Cleanup(func() { Deps.SandboxRuntime = previous })

	router := gin.New()
	router.GET("/status", HandleSandboxRuntimeStatus)
	router.GET("/detect", HandleSandboxRuntimeDetect)
	router.GET("/active", HandleSandboxRuntimeActive)

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/status", want: http.StatusOK},
		{path: "/detect?pid=42", want: http.StatusOK},
		{path: "/active?limit=4", want: http.StatusOK},
		{path: "/detect?pid=0", want: http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s status=%d body=%s want=%d", tc.path, rec.Code, rec.Body.String(), tc.want)
		}
	}
}
