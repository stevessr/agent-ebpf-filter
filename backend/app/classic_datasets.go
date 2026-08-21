package app

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// ---- classic dataset management ----

// ClassicDataset holds a small built-in dataset with preprocessing and cached loads.
type ClassicDataset struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Features    [][]float64               `json:"features"`
	Labels      []string                  `json:"labels"`
	Metadata    map[string]any            `json:"metadata,omitempty"`
	Statistics  map[string]float64        `json:"statistics,omitempty"`
	Encodings   map[string]map[string]int `json:"encodings,omitempty"`
}

// DatasetLoader loads a classic dataset by name.
type DatasetLoader interface {
	Name() string
	Load() (*ClassicDataset, error)
}

type classicDatasetLoaderFunc struct {
	name string
	load func() (*ClassicDataset, error)
}

func (l classicDatasetLoaderFunc) Name() string                   { return l.name }
func (l classicDatasetLoaderFunc) Load() (*ClassicDataset, error) { return l.load() }

var (
	classicDatasetCacheMu sync.RWMutex
	classicDatasetCache   = make(map[string]*ClassicDataset)
	classicDatasetLoaders = map[string]DatasetLoader{
		"iris":          classicDatasetLoaderFunc{name: "iris", load: loadIrisDataset},
		"wine":          classicDatasetLoaderFunc{name: "wine", load: loadWineDataset},
		"breast_cancer": classicDatasetLoaderFunc{name: "breast_cancer", load: loadBreastCancerDataset},
		"breast-cancer": classicDatasetLoaderFunc{name: "breast-cancer", load: loadBreastCancerDataset},
		"breast cancer": classicDatasetLoaderFunc{name: "breast cancer", load: loadBreastCancerDataset},
	}
)

func registerClassicDatasetRoutes(r gin.IRouter) {
	r.GET("/config/ml/datasets/classic", handleClassicDatasetsListGet)
	r.GET("/config/ml/datasets/classic/:name", handleClassicDatasetGet)
	r.POST("/config/ml/datasets/classic/:name/preview", handleClassicDatasetPreviewPost)
}

func handleClassicDatasetsListGet(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"datasets": classicDatasetNames()})
}

func handleClassicDatasetGet(c *gin.Context) {
	name := normalizeClassicDatasetName(c.Param("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dataset name is required"})
		return
	}
	ds, err := loadClassicDataset(name)
	if err != nil {
		handleClassicDatasetError(c, err)
		return
	}
	c.JSON(http.StatusOK, ds)
}

func handleClassicDatasetPreviewPost(c *gin.Context) {
	name := normalizeClassicDatasetName(c.Param("name"))
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dataset name is required"})
		return
	}
	var req struct {
		Normalize   bool   `json:"normalize"`
		Standardize bool   `json:"standardize"`
		Encode      string `json:"encode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	ds, err := loadClassicDataset(name)
	if err != nil {
		handleClassicDatasetError(c, err)
		return
	}
	processed, err := preprocessClassicDataset(ds, req.Normalize, req.Standardize, req.Encode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, processed)
}

func handleClassicDatasetError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errClassicDatasetNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, errClassicDatasetInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

var (
	errClassicDatasetNotFound = errors.New("classic dataset not found")
	errClassicDatasetInvalid  = errors.New("classic dataset is invalid")
)

func classicDatasetNames() []string {
	names := make([]string, 0, len(classicDatasetLoaders))
	seen := make(map[string]struct{})
	for name := range classicDatasetLoaders {
		n := canonicalClassicDatasetName(name)
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func normalizeClassicDatasetName(raw string) string {
	return canonicalClassicDatasetName(raw)
}

func canonicalClassicDatasetName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "_", "-")
	raw = strings.ReplaceAll(raw, " ", "-")
	switch raw {
	case "breast-cancer", "breastcancer", "bc":
		return "breast_cancer"
	default:
		return raw
	}
}

func loadClassicDataset(name string) (*ClassicDataset, error) {
	name = canonicalClassicDatasetName(name)
	if name == "" {
		return nil, fmt.Errorf("%w: empty name", errClassicDatasetInvalid)
	}

	classicDatasetCacheMu.RLock()
	if cached, ok := classicDatasetCache[name]; ok && cached != nil {
		classicDatasetCacheMu.RUnlock()
		return cloneClassicDataset(cached), nil
	}
	classicDatasetCacheMu.RUnlock()

	loader, ok := classicDatasetLoaders[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errClassicDatasetNotFound, name)
	}
	ds, err := loader.Load()
	if err != nil {
		return nil, err
	}
	if err := validateClassicDataset(ds); err != nil {
		return nil, err
	}

	classicDatasetCacheMu.Lock()
	classicDatasetCache[name] = cloneClassicDataset(ds)
	classicDatasetCacheMu.Unlock()
	return cloneClassicDataset(ds), nil
}

func validateClassicDataset(ds *ClassicDataset) error {
	if ds == nil {
		return fmt.Errorf("%w: nil dataset", errClassicDatasetInvalid)
	}
	if strings.TrimSpace(ds.Name) == "" {
		return fmt.Errorf("%w: missing name", errClassicDatasetInvalid)
	}
	if len(ds.Features) == 0 || len(ds.Labels) == 0 || len(ds.Features) != len(ds.Labels) {
		return fmt.Errorf("%w: inconsistent feature/label dimensions", errClassicDatasetInvalid)
	}
	cols := len(ds.Features[0])
	if cols == 0 {
		return fmt.Errorf("%w: empty feature matrix", errClassicDatasetInvalid)
	}
	for i := range ds.Features {
		if len(ds.Features[i]) != cols {
			return fmt.Errorf("%w: ragged feature matrix", errClassicDatasetInvalid)
		}
		for j := range ds.Features[i] {
			if math.IsNaN(ds.Features[i][j]) || math.IsInf(ds.Features[i][j], 0) {
				return fmt.Errorf("%w: non-finite value at row %d col %d", errClassicDatasetInvalid, i, j)
			}
		}
	}
	return nil
}

func cloneClassicDataset(ds *ClassicDataset) *ClassicDataset {
	if ds == nil {
		return nil
	}
	clone := *ds
	clone.Features = make([][]float64, len(ds.Features))
	for i := range ds.Features {
		clone.Features[i] = append([]float64(nil), ds.Features[i]...)
	}
	clone.Labels = append([]string(nil), ds.Labels...)
	if ds.Metadata != nil {
		clone.Metadata = make(map[string]any, len(ds.Metadata))
		for k, v := range ds.Metadata {
			clone.Metadata[k] = v
		}
	}
	if ds.Statistics != nil {
		clone.Statistics = make(map[string]float64, len(ds.Statistics))
		for k, v := range ds.Statistics {
			clone.Statistics[k] = v
		}
	}
	if ds.Encodings != nil {
		clone.Encodings = make(map[string]map[string]int, len(ds.Encodings))
		for k, enc := range ds.Encodings {
			clone.Encodings[k] = make(map[string]int, len(enc))
			for kk, vv := range enc {
				clone.Encodings[k][kk] = vv
			}
		}
	}
	return &clone
}

func preprocessClassicDataset(ds *ClassicDataset, normalize, standardize bool, encode string) (*ClassicDataset, error) {
	if ds == nil {
		return nil, fmt.Errorf("%w: nil dataset", errClassicDatasetInvalid)
	}
	out := cloneClassicDataset(ds)
	if out == nil {
		return nil, fmt.Errorf("%w: clone failed", errClassicDatasetInvalid)
	}

	if encode != "" && encode != "none" && encode != "label" {
		return nil, fmt.Errorf("unsupported encoding mode %q", encode)
	}
	if normalize && standardize {
		return nil, fmt.Errorf("normalization and standardization are mutually exclusive")
	}
	if normalize {
		out.Features = minMaxNormalize(out.Features)
	}
	if standardize {
		var err error
		out.Features, err = zScoreStandardize(out.Features)
		if err != nil {
			return nil, err
		}
	}
	if encode == "label" {
		encoded, encodings := labelEncodeDataset(out.Labels)
		out.Labels = encoded
		out.Encodings = map[string]map[string]int{"labels": encodings}
	}
	stats := computeClassicDatasetStats(out.Features)
	out.Statistics = stats
	return out, nil
}

func computeClassicDatasetStats(features [][]float64) map[string]float64 {
	stats := map[string]float64{}
	if len(features) == 0 || len(features[0]) == 0 {
		return stats
	}
	rows, cols := len(features), len(features[0])
	for c := 0; c < cols; c++ {
		minV, maxV := features[0][c], features[0][c]
		sum := 0.0
		for r := 0; r < rows; r++ {
			v := features[r][c]
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
			sum += v
		}
		stats[fmt.Sprintf("col_%d_min", c)] = minV
		stats[fmt.Sprintf("col_%d_max", c)] = maxV
		stats[fmt.Sprintf("col_%d_mean", c)] = sum / float64(rows)
	}
	return stats
}

func minMaxNormalize(features [][]float64) [][]float64 {
	if len(features) == 0 {
		return nil
	}
	rows, cols := len(features), len(features[0])
	mins := make([]float64, cols)
	maxs := make([]float64, cols)
	copy(mins, features[0])
	copy(maxs, features[0])
	for r := 1; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if features[r][c] < mins[c] {
				mins[c] = features[r][c]
			}
			if features[r][c] > maxs[c] {
				maxs[c] = features[r][c]
			}
		}
	}
	out := make([][]float64, rows)
	for r := 0; r < rows; r++ {
		out[r] = make([]float64, cols)
		for c := 0; c < cols; c++ {
			d := maxs[c] - mins[c]
			if d == 0 {
				out[r][c] = 0
			} else {
				out[r][c] = (features[r][c] - mins[c]) / d
			}
		}
	}
	return out
}

func zScoreStandardize(features [][]float64) ([][]float64, error) {
	if len(features) == 0 {
		return nil, fmt.Errorf("empty dataset")
	}
	rows, cols := len(features), len(features[0])
	means := make([]float64, cols)
	for _, row := range features {
		if len(row) != cols {
			return nil, fmt.Errorf("ragged feature matrix")
		}
		for c, v := range row {
			means[c] += v
		}
	}
	for c := range means {
		means[c] /= float64(rows)
	}
	stds := make([]float64, cols)
	for _, row := range features {
		for c, v := range row {
			d := v - means[c]
			stds[c] += d * d
		}
	}
	for c := range stds {
		stds[c] = math.Sqrt(stds[c] / float64(rows))
	}
	out := make([][]float64, rows)
	for r := 0; r < rows; r++ {
		out[r] = make([]float64, cols)
		for c := 0; c < cols; c++ {
			if stds[c] == 0 {
				out[r][c] = 0
			} else {
				out[r][c] = (features[r][c] - means[c]) / stds[c]
			}
		}
	}
	return out, nil
}

func labelEncodeDataset(labels []string) ([]string, map[string]int) {
	mapping := make(map[string]int)
	encoded := make([]string, len(labels))
	for i, label := range labels {
		if _, ok := mapping[label]; !ok {
			mapping[label] = len(mapping)
		}
		encoded[i] = fmt.Sprintf("%d", mapping[label])
	}
	return encoded, mapping
}

func loadIrisDataset() (*ClassicDataset, error) {
	return &ClassicDataset{
		Name:        "iris",
		Description: "Iris flower classification dataset",
		Features:    [][]float64{{5.1, 3.5, 1.4, 0.2}, {4.9, 3.0, 1.4, 0.2}, {6.2, 3.4, 5.4, 2.3}, {5.9, 3.0, 5.1, 1.8}},
		Labels:      []string{"setosa", "setosa", "virginica", "virginica"},
		Metadata:    map[string]any{"classes": 3, "features": 4, "samples": 4},
	}, nil
}

func loadWineDataset() (*ClassicDataset, error) {
	return &ClassicDataset{
		Name:        "wine",
		Description: "Wine recognition dataset",
		Features:    [][]float64{{14.23, 1.71, 2.43}, {13.20, 1.78, 2.14}, {12.37, 1.17, 1.92}},
		Labels:      []string{"1", "1", "2"},
		Metadata:    map[string]any{"classes": 3, "features": 3, "samples": 3},
	}, nil
}

func loadBreastCancerDataset() (*ClassicDataset, error) {
	return &ClassicDataset{
		Name:        "breast_cancer",
		Description: "Breast cancer diagnostic dataset",
		Features:    [][]float64{{17.99, 10.38, 122.8}, {20.57, 17.77, 132.9}, {19.69, 21.25, 130.0}},
		Labels:      []string{"malignant", "malignant", "benign"},
		Metadata:    map[string]any{"classes": 2, "features": 3, "samples": 3},
	}, nil
}

func LoadClassicDataset(name string) (*ClassicDataset, error) {
	return loadClassicDataset(name)
}
