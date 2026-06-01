package main

type sweepProfile struct {
	Name       string
	ModelType  ModelType
	Comparable bool
	Kind       string // "heatmap" or "bar"
	XName      string
	YName      string
	XValues    []int
	YValues    []int
	XLabel     func(int) string
	YLabel     func(int) string
	Build      func(x, y int) MLConfig
	Summary    func(cfg MLConfig) string

	// Parameter metadata is used by the comprehensive axis-sweep verifier.
	// Numeric parameters must expose RequiredDiscretePoints unique values; small
	// categorical parameters use ParameterKind="categorical" and a lower/zero
	// requirement because there are only a few meaningful choices.
	ParameterName          string
	ParameterKind          string
	RequiredDiscretePoints int
	DatasetName            string
	DatasetDescription     string
}

type sweepResult struct {
	Profile             string
	Dataset             string
	BaseProfile         string
	ModelType           ModelType
	ParameterName       string
	ParameterKind       string
	RequiredPoints      int
	ConfiguredPoints    int
	XValue              int
	YValue              int
	ConfigSummary       string
	TrainAccuracy       float64
	ValidationAccuracy  float64
	AllowPassRate       float64
	NumSamples          int
	TrainSamples        int
	ValidationSamples   int
	Duration            float64
	InferenceDuration   float64
	InferenceSamples    int
	InferenceLatencyMs  float64
	InferenceThroughput float64
	MemoryBytes         int64
	Error               string
}

type profileSummary struct {
	Profile sweepProfile
	Best    sweepResult
	Results []sweepResult
	Chart   string
}

type sweepDataset struct {
	Name        string
	Description string
	Samples     []TrainingSample
}

type repeatRunResult struct {
	Profile             string
	ModelType           ModelType
	XValue              int
	YValue              int
	ConfigSummary       string
	RunIndex            int
	TrainAccuracy       float64
	ValidationAccuracy  float64
	AllowPassRate       float64
	NumSamples          int
	TrainSamples        int
	ValidationSamples   int
	Duration            float64
	InferenceDuration   float64
	InferenceSamples    int
	InferenceLatencyMs  float64
	InferenceThroughput float64
	MemoryBytes         int64
	Error               string
}

type repeatSummary struct {
	Profile              string
	ModelType            ModelType
	Comparable           bool
	XValue               int
	YValue               int
	ConfigSummary        string
	Runs                 int
	SuccessRuns          int
	FailureRuns          int
	TrainMean            float64
	TrainStd             float64
	ValidationMean       float64
	ValidationStd        float64
	ValidationMin        float64
	ValidationMax        float64
	AllowMean            float64
	AllowStd             float64
	AllowMin             float64
	AllowMax             float64
	DurationMean         float64
	DurationStd          float64
	InferenceMean        float64
	InferenceStd         float64
	InferenceMin         float64
	InferenceMax         float64
	InferenceLatencyMean float64
	InferenceLatencyStd  float64
	TrainMin             float64
	TrainMax             float64
	MemoryMean           float64
	MemoryStd            float64
	MemoryMin            float64
	MemoryMax            float64
	SuccessRate          float64
}

type stabilityTask struct {
	Profile          sweepProfile
	Config           sweepResult
	Store            *TrainingDataStore
	BenchmarkSamples []TrainingSample
}
