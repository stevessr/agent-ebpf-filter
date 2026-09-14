package app

import (
	"agent-ebpf-filter/app/captureprofile"
	"agent-ebpf-filter/app/platform"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const captureProfileOverlayFilename = "api-capture-profiles.json"
const maxCaptureProfilePreviewObservations = 500

type captureProfileCapabilities struct {
	Sources         []string `json:"sources"`
	Protocols       []string `json:"protocols"`
	Directions      []string `json:"directions"`
	Methods         []string `json:"methods"`
	PrivacyBoundary string   `json:"privacyBoundary"`
	MaxPreviewFlows int      `json:"maxPreviewFlows"`
}

type captureProfileStateResponse struct {
	Path              string                     `json:"path"`
	CustomProfiles    []captureprofile.Profile   `json:"customProfiles"`
	EffectiveProfiles []captureprofile.Profile   `json:"effectiveProfiles"`
	BuiltinCount      int                        `json:"builtinCount"`
	CustomCount       int                        `json:"customCount"`
	EffectiveCount    int                        `json:"effectiveCount"`
	UpdatedAt         string                     `json:"updatedAt,omitempty"`
	Capabilities      captureProfileCapabilities `json:"capabilities"`
}

type captureProfileUpdateRequest struct {
	Profiles []captureprofile.Profile `json:"profiles"`
}

type captureProfilePreviewRequest struct {
	Profile      captureprofile.Profile       `json:"profile"`
	Observations []captureprofile.Observation `json:"observations"`
}

type captureProfilePreviewMatch struct {
	Index int                  `json:"index"`
	Match captureprofile.Match `json:"match"`
}

type captureProfilePreviewResponse struct {
	Profile      captureprofile.Profile       `json:"profile"`
	MatchedCount int                          `json:"matchedCount"`
	Matches      []captureProfilePreviewMatch `json:"matches"`
}

func captureProfileOverlayPath() string {
	if configured := strings.TrimSpace(os.Getenv("AGENT_EBPF_API_PROFILES")); configured != "" {
		return filepath.Clean(configured)
	}
	return filepath.Join(platform.RuntimeSettingsDir(), captureProfileOverlayFilename)
}

func captureProfileCapabilitiesValue() captureProfileCapabilities {
	return captureProfileCapabilities{
		Sources:         []string{"kernel_socket_prefix", "tls_plaintext"},
		Protocols:       []string{"http1", "http2", "grpc"},
		Directions:      []string{"outgoing", "incoming"},
		Methods:         []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT"},
		PrivacyBoundary: "metadata-only: request/response start-line, host/path/method and header names; no body, credentials or query secrets",
		MaxPreviewFlows: maxCaptureProfilePreviewObservations,
	}
}

func validateCaptureProfileOverlay(profiles []captureprofile.Profile) ([]captureprofile.Profile, error) {
	registry := captureprofile.NewRegistry(nil)
	if err := registry.Replace(profiles); err != nil {
		return nil, err
	}
	return registry.Profiles(), nil
}

func ensureCaptureProfileOverlayFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("API capture profile path is empty")
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat API capture profile file: %w", err)
	}
	if err := platform.MkdirAllAsRealUser(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create API capture profile directory: %w", err)
	}
	if err := platform.WriteFileAsRealUser(path, []byte("[]\n"), 0644); err != nil {
		return fmt.Errorf("initialize API capture profile file: %w", err)
	}
	return nil
}

func readCaptureProfileOverlay(path string) ([]captureprofile.Profile, error) {
	if err := ensureCaptureProfileOverlayFile(path); err != nil {
		return nil, err
	}
	return captureprofile.LoadJSON(path)
}

func persistCaptureProfileOverlay(path string, profiles []captureprofile.Profile) error {
	normalized, err := validateCaptureProfileOverlay(profiles)
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("encode API capture profiles: %w", err)
	}
	payload = append(payload, '\n')
	if err := platform.MkdirAllAsRealUser(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create API capture profile directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := platform.WriteFileAsRealUser(tmp, payload, 0644); err != nil {
		return fmt.Errorf("write API capture profile staging file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publish API capture profiles atomically: %w", err)
	}
	if err := captureprofile.ReloadDefaultJSON(path); err != nil {
		return fmt.Errorf("publish API capture profile registry: %w", err)
	}
	return nil
}

func buildCaptureProfileState(path string) (captureProfileStateResponse, error) {
	custom, err := readCaptureProfileOverlay(path)
	if err != nil {
		return captureProfileStateResponse{}, err
	}
	updatedAt := ""
	if info, err := os.Stat(path); err == nil {
		updatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	builtin := captureprofile.BuiltinProfiles()
	effective := captureprofile.Default.Profiles()
	return captureProfileStateResponse{
		Path:              path,
		CustomProfiles:    custom,
		EffectiveProfiles: effective,
		BuiltinCount:      len(builtin),
		CustomCount:       len(custom),
		EffectiveCount:    len(effective),
		UpdatedAt:         updatedAt,
		Capabilities:      captureProfileCapabilitiesValue(),
	}, nil
}

func previewCaptureProfile(profile captureprofile.Profile, observations []captureprofile.Observation) (captureProfilePreviewResponse, error) {
	if len(observations) > maxCaptureProfilePreviewObservations {
		return captureProfilePreviewResponse{}, fmt.Errorf("preview accepts at most %d observations", maxCaptureProfilePreviewObservations)
	}
	registry := captureprofile.NewRegistry(nil)
	if err := registry.Replace([]captureprofile.Profile{profile}); err != nil {
		return captureProfilePreviewResponse{}, err
	}
	normalized := registry.Profiles()[0]
	matches := make([]captureProfilePreviewMatch, 0)
	for index, observation := range observations {
		if match, ok := registry.Match(observation); ok {
			matches = append(matches, captureProfilePreviewMatch{Index: index, Match: match})
		}
	}
	return captureProfilePreviewResponse{
		Profile:      normalized,
		MatchedCount: len(matches),
		Matches:      matches,
	}, nil
}

func handleGetCaptureProfiles(c *gin.Context) {
	state, err := buildCaptureProfileState(captureProfileOverlayPath())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, state)
}

func handlePutCaptureProfiles(c *gin.Context) {
	var request captureProfileUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid capture profile payload: " + err.Error()})
		return
	}
	path := captureProfileOverlayPath()
	if err := persistCaptureProfileOverlay(path, request.Profiles); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	state, err := buildCaptureProfileState(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, state)
}

func handlePreviewCaptureProfile(c *gin.Context) {
	var request captureProfilePreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid capture profile preview payload: " + err.Error()})
		return
	}
	response, err := previewCaptureProfile(request.Profile, request.Observations)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}
