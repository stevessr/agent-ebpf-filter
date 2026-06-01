package app

import (
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section autotunetypes.go ----

// MLAutoTuneRequest describes a grid-search auto-tune request.
type MLAutoTuneRequest struct {
	XAxis                string  `json:"xAxis"`
	YAxis                string  `json:"yAxis"`
	GridSize             int     `json:"gridSize"`
	Granularity          float64 `json:"granularity"`
	Metric               string  `json:"metric"`
	ValidationSplitRatio float64 `json:"validationSplitRatio"`
	MinX                 *int    `json:"minX,omitempty"`
	MaxX                 *int    `json:"maxX,omitempty"`
	MinY                 *int    `json:"minY,omitempty"`
	MaxY                 *int    `json:"maxY,omitempty"`
}

// MLAutoTuneCell records the result of evaluating one grid cell.
type MLAutoTuneCell struct {
	XIndex               int     `json:"xIndex"`
	YIndex               int     `json:"yIndex"`
	XValue               int     `json:"xValue"`
	YValue               int     `json:"yValue"`
	NumTrees             int     `json:"numTrees"`
	MaxDepth             int     `json:"maxDepth"`
	MinSamplesLeaf       int     `json:"minSamplesLeaf"`
	TrainAccuracy        float64 `json:"trainAccuracy"`
	ValidationAccuracy   float64 `json:"validationAccuracy"`
	InferenceThroughput  float64 `json:"inferenceThroughput"`
	InferenceMsPerSample float64 `json:"inferenceMsPerSample"`
	TrainDuration        float64 `json:"trainDuration"`
	EvalDuration         float64 `json:"evalDuration"`
	Score                float64 `json:"score"`
}

// MLAutoTuneResponse is the full result of an auto-tune grid search.
type MLAutoTuneResponse struct {
	XAxis           string           `json:"xAxis"`
	YAxis           string           `json:"yAxis"`
	Metric          string           `json:"metric"`
	Granularity     float64          `json:"granularity"`
	GridSize        int              `json:"gridSize"`
	XValues         []int            `json:"xValues"`
	YValues         []int            `json:"yValues"`
	SampleCount     int              `json:"sampleCount"`
	ValidationCount int              `json:"validationCount"`
	TotalDuration   float64          `json:"totalDuration"`
	Cells           []MLAutoTuneCell `json:"cells"`
	Best            *MLAutoTuneCell  `json:"best,omitempty"`
}

// MLModelTuneRequest describes a cross-model auto-tune request.
type MLModelTuneRequest struct {
	ModelTypes           []string `json:"modelTypes"`
	Metric               string   `json:"metric"`
	ValidationSplitRatio float64  `json:"validationSplitRatio"`
	TuneParams           bool     `json:"tuneParams"`
	ApplyBest            bool     `json:"applyBest"`
	XAxis                string   `json:"xAxis"`
	YAxis                string   `json:"yAxis"`
	GridSize             int      `json:"gridSize"`
	Granularity          float64  `json:"granularity"`
	MinX                 *int     `json:"minX,omitempty"`
	MaxX                 *int     `json:"maxX,omitempty"`
	MinY                 *int     `json:"minY,omitempty"`
	MaxY                 *int     `json:"maxY,omitempty"`
}

// MLModelTuneCandidate records the result of evaluating one model type.
type MLModelTuneCandidate struct {
	ModelType            string              `json:"modelType"`
	Label                string              `json:"label"`
	Base                 string              `json:"base"`
	Recommended          bool                `json:"recommended,omitempty"`
	HyperParams          map[string]int      `json:"hyperParams"`
	TrainAccuracy        float64             `json:"trainAccuracy"`
	ValidationAccuracy   float64             `json:"validationAccuracy"`
	InferenceThroughput  float64             `json:"inferenceThroughput"`
	InferenceMsPerSample float64             `json:"inferenceMsPerSample"`
	TrainDuration        float64             `json:"trainDuration"`
	EvalDuration         float64             `json:"evalDuration"`
	Score                float64             `json:"score"`
	SampleCount          int                 `json:"sampleCount"`
	ValidationCount      int                 `json:"validationCount"`
	ParamTune            *MLAutoTuneResponse `json:"paramTune,omitempty"`
	Applied              bool                `json:"applied,omitempty"`
	Error                string              `json:"error,omitempty"`
}

// MLModelTuneResponse is the full result of a cross-model auto-tune.
type MLModelTuneResponse struct {
	Metric          string                 `json:"metric"`
	SampleCount     int                    `json:"sampleCount"`
	ValidationCount int                    `json:"validationCount"`
	TotalDuration   float64                `json:"totalDuration"`
	Candidates      []MLModelTuneCandidate `json:"candidates"`
	Best            *MLModelTuneCandidate  `json:"best,omitempty"`
}

// MLAutoTuneState is the serialisable progress snapshot for auto-tune jobs.
type MLAutoTuneState struct {
	JobID       string               `json:"jobId"`
	Mode        string               `json:"mode,omitempty"`
	Running     bool                 `json:"running"`
	Progress    float64              `json:"progress"`
	Completed   int                  `json:"completed"`
	Total       int                  `json:"total"`
	Message     string               `json:"message,omitempty"`
	Error       string               `json:"error,omitempty"`
	StartedAt   string               `json:"startedAt,omitempty"`
	FinishedAt  string               `json:"finishedAt,omitempty"`
	Result      *MLAutoTuneResponse  `json:"result,omitempty"`
	ModelResult *MLModelTuneResponse `json:"modelResult,omitempty"`
}

// autoTuneRuntime holds the mutable state for the current auto-tune job.
type autoTuneRuntime struct {
	mu          sync.RWMutex
	state       MLAutoTuneState
	result      *MLAutoTuneResponse
	modelResult *MLModelTuneResponse
}

var globalAutoTuneState = &autoTuneRuntime{}

func (s *autoTuneRuntime) begin(jobID string, total int, message string) {
	s.beginMode(jobID, "params", total, message)
}

func (s *autoTuneRuntime) beginMode(jobID, mode string, total int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = MLAutoTuneState{
		JobID:     jobID,
		Mode:      mode,
		Running:   true,
		Progress:  0,
		Completed: 0,
		Total:     total,
		Message:   message,
		StartedAt: time.Now().Format(time.RFC3339),
	}
	s.result = nil
	s.modelResult = nil
}

func (s *autoTuneRuntime) tryBegin(jobID string, total int, message string) bool {
	return s.tryBeginMode(jobID, "params", total, message)
}

func (s *autoTuneRuntime) tryBeginMode(jobID, mode string, total int, message string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Running {
		return false
	}
	s.state = MLAutoTuneState{
		JobID:     jobID,
		Mode:      mode,
		Running:   true,
		Progress:  0,
		Completed: 0,
		Total:     total,
		Message:   message,
		StartedAt: time.Now().Format(time.RFC3339),
	}
	s.result = nil
	s.modelResult = nil
	return true
}

func (s *autoTuneRuntime) update(jobID string, completed, total int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jobID != "" && s.state.JobID != "" && s.state.JobID != jobID {
		return
	}
	s.state.JobID = jobID
	s.state.Running = true
	s.state.Completed = completed
	s.state.Total = total
	if total > 0 {
		s.state.Progress = mathMax0Min1(float64(completed) / float64(total))
	} else {
		s.state.Progress = 0
	}
	s.state.Message = message
	s.state.Error = ""
}

func (s *autoTuneRuntime) setError(jobID string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Running = false
	s.state.Error = errMsg
	if s.state.Mode == "models" {
		s.state.Message = "模型调优失败"
	} else {
		s.state.Message = "调优失败"
	}
}

func autoTuneBestScore(resp *MLAutoTuneResponse) float64 {
	if resp == nil || resp.Best == nil {
		return 0
	}
	return resp.Best.Score
}

func (s *autoTuneRuntime) finish(jobID string, result *MLAutoTuneResponse, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jobID != "" && s.state.JobID != "" && s.state.JobID != jobID {
		return
	}
	s.state.Running = false
	s.state.FinishedAt = time.Now().Format(time.RFC3339)
	if err != nil {
		s.state.Error = err.Error()
		s.state.Message = "自动调参失败"
		s.result = nil
		s.state.Result = nil
		return
	}
	s.state.Mode = "params"
	s.state.Progress = 1
	if result != nil {
		s.result = result
		s.state.Result = result
	}
	s.state.Error = ""
	if s.state.Message == "" {
		s.state.Message = "自动调参完成"
	}
}

func (s *autoTuneRuntime) finishModelTune(jobID string, result *MLModelTuneResponse, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jobID != "" && s.state.JobID != "" && s.state.JobID != jobID {
		return
	}
	s.state.Mode = "models"
	s.state.Running = false
	s.state.FinishedAt = time.Now().Format(time.RFC3339)
	if err != nil {
		s.state.Error = err.Error()
		s.state.Message = "模型调优失败"
		s.modelResult = nil
		s.state.ModelResult = nil
		return
	}
	s.state.Progress = 1
	if result != nil {
		s.modelResult = result
		s.state.ModelResult = result
	}
	s.state.Error = ""
	if s.state.Message == "" {
		s.state.Message = "模型调优完成"
	}
}

func (s *autoTuneRuntime) snapshot() MLAutoTuneState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.state
	if s.result != nil {
		state.Result = s.result
	}
	if s.modelResult != nil {
		state.ModelResult = s.modelResult
	}
	return state
}

// mathMax0Min1 clamps v to [0, 1].
func mathMax0Min1(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
