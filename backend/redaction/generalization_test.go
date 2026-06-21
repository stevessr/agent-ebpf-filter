package redaction

import (
	"strings"
	"testing"
	"time"
)

func TestGeneralizeIP_IPv4(t *testing.T) {
	testCases := []struct {
		name      string
		precision IPPrecisionLevel
		input     string
		expected  string
	}{
		{"full", IPPrecisionFull, "192.168.1.100", "192.168.1.100"},
		{"subnet", IPPrecisionSubnet, "192.168.1.100", "192.168.1.0/24"},
		{"class", IPPrecisionClass, "192.168.1.100", "192.168.0.0/16"},
		{"none", IPPrecisionNone, "192.168.1.100", "[IP_GENERALIZED]"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGeneralizer(GeneralizationConfig{
				IPPrecision: tc.precision,
				Enabled:     true,
			})

			result := g.GeneralizeIP(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGeneralizeIP_PrivateAddresses(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		IPPrecision: IPPrecisionSubnet,
		Enabled:     true,
	})

	testCases := []struct {
		input    string
		expected string
	}{
		{"10.0.0.5", "10.0.0.0/24"},
		{"172.16.5.10", "172.16.5.0/24"},
		{"192.168.254.200", "192.168.254.0/24"},
	}

	for _, tc := range testCases {
		result := g.GeneralizeIP(tc.input)
		if result != tc.expected {
			t.Errorf("For %s: expected %s, got %s", tc.input, tc.expected, result)
		}
	}
}

func TestGeneralizeIP_Invalid(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		IPPrecision: IPPrecisionSubnet,
		Enabled:     true,
	})

	invalid := "not-an-ip"
	result := g.GeneralizeIP(invalid)

	if result != invalid {
		t.Errorf("Invalid IP should be returned as-is, got: %s", result)
	}
}

func TestGeneralizeTimestamp(t *testing.T) {
	ts := time.Date(2026, 6, 8, 14, 35, 47, 123456789, time.UTC)

	testCases := []struct {
		name      string
		precision TimePrecisionLevel
		expected  time.Time
	}{
		{
			"full",
			TimePrecisionFull,
			ts,
		},
		{
			"minute",
			TimePrecisionMinute,
			time.Date(2026, 6, 8, 14, 35, 0, 0, time.UTC),
		},
		{
			"hour",
			TimePrecisionHour,
			time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
		},
		{
			"day",
			TimePrecisionDay,
			time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC),
		},
		{
			"month",
			TimePrecisionMonth,
			time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGeneralizer(GeneralizationConfig{
				TimePrecision: tc.precision,
				Enabled:       true,
			})

			result := g.GeneralizeTimestamp(ts)
			if !result.Equal(tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGeneralizePath(t *testing.T) {
	testCases := []struct {
		name   string
		level  PathGeneralizationLevel
		input  string
		expect string
	}{
		{
			"full",
			PathGeneralizationFull,
			"/home/alice/projects/myapp/src/main.go",
			"/home/alice/projects/myapp/src/main.go",
		},
		{
			"pattern",
			PathGeneralizationPattern,
			"/home/alice/projects/myapp/src/main.go",
			"/home/*/projects/myapp/src/main.go",
		},
		{
			"base",
			PathGeneralizationBase,
			"/home/alice/projects/myapp/src/main.go",
			"main.go",
		},
		{
			"none",
			PathGeneralizationNone,
			"/home/alice/projects/myapp/src/main.go",
			"[PATH_GENERALIZED]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGeneralizer(GeneralizationConfig{
				PathGeneralization: tc.level,
				Enabled:            true,
			})

			result := g.GeneralizePath(tc.input)
			if result != tc.expect {
				t.Errorf("Expected %s, got %s", tc.expect, result)
			}
		})
	}
}

func TestGeneralizePath_MacOS(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		PathGeneralization: PathGeneralizationPattern,
		Enabled:            true,
	})

	input := "/Users/bob/Documents/report.pdf"
	result := g.GeneralizePath(input)

	expected := "/Users/*/Documents/report.pdf"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGeneralizePath_ConfigDir(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		PathGeneralization: PathGeneralizationPattern,
		Enabled:            true,
	})

	input := "/home/user/.config/app/settings.json"
	result := g.GeneralizePath(input)

	// Should generalize both /home/user and .config/app
	if !contains(result, "/home/*/") || !contains(result, ".config/*/") {
		t.Errorf("Path not properly generalized: %s", result)
	}
}

func TestGeneralizeBatch_IP(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		IPPrecision: IPPrecisionSubnet,
		Enabled:     true,
	})

	ips := []string{
		"192.168.1.1",
		"192.168.1.2",
		"10.0.0.5",
	}

	results := g.GeneralizeBatch(ips, "ip")

	expected := []string{
		"192.168.1.0/24",
		"192.168.1.0/24", // Same subnet
		"10.0.0.0/24",
	}

	for i, result := range results {
		if result != expected[i] {
			t.Errorf("Index %d: expected %s, got %s", i, expected[i], result)
		}
	}
}

func TestGeneralizeBatch_Path(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		PathGeneralization: PathGeneralizationBase,
		Enabled:            true,
	})

	paths := []string{
		"/home/alice/file1.txt",
		"/home/bob/file2.txt",
		"/var/log/app.log",
	}

	results := g.GeneralizeBatch(paths, "path")

	expected := []string{
		"file1.txt",
		"file2.txt",
		"app.log",
	}

	for i, result := range results {
		if result != expected[i] {
			t.Errorf("Index %d: expected %s, got %s", i, expected[i], result)
		}
	}
}

func TestGeneralizer_Disabled(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		IPPrecision:   IPPrecisionNone,
		Enabled:       false,
	})

	ip := "192.168.1.100"
	result := g.GeneralizeIP(ip)

	if result != ip {
		t.Error("Disabled generalizer should return original value")
	}
}

func TestDefaultGeneralizationConfig(t *testing.T) {
	config := DefaultGeneralizationConfig()

	if config.IPPrecision != IPPrecisionSubnet {
		t.Errorf("Expected subnet precision, got %s", config.IPPrecision)
	}
	if config.TimePrecision != TimePrecisionHour {
		t.Errorf("Expected hour precision, got %s", config.TimePrecision)
	}
	if config.PathGeneralization != PathGeneralizationPattern {
		t.Errorf("Expected pattern generalization, got %s", config.PathGeneralization)
	}
	if !config.Enabled {
		t.Error("Default config should be enabled")
	}
}

func TestStrictGeneralizationConfig(t *testing.T) {
	config := StrictGeneralizationConfig()

	if config.IPPrecision != IPPrecisionClass {
		t.Errorf("Expected class precision, got %s", config.IPPrecision)
	}
	if config.TimePrecision != TimePrecisionDay {
		t.Errorf("Expected day precision, got %s", config.TimePrecision)
	}
	if config.PathGeneralization != PathGeneralizationBase {
		t.Errorf("Expected base generalization, got %s", config.PathGeneralization)
	}
	if !config.Enabled {
		t.Error("Strict config should be enabled")
	}
}

func TestGeneralizeIP_Consistency(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		IPPrecision: IPPrecisionSubnet,
		Enabled:     true,
	})

	ip := "192.168.50.75"

	// Multiple calls should return consistent results
	result1 := g.GeneralizeIP(ip)
	result2 := g.GeneralizeIP(ip)

	if result1 != result2 {
		t.Errorf("Generalization not consistent: %s != %s", result1, result2)
	}
}

func TestGeneralizeTimestamp_Consistency(t *testing.T) {
	g := NewGeneralizer(GeneralizationConfig{
		TimePrecision: TimePrecisionHour,
		Enabled:       true,
	})

	ts := time.Date(2026, 6, 8, 14, 35, 47, 0, time.UTC)

	result1 := g.GeneralizeTimestamp(ts)
	result2 := g.GeneralizeTimestamp(ts)

	if !result1.Equal(result2) {
		t.Error("Timestamp generalization not consistent")
	}
}

func BenchmarkGeneralizeIP(b *testing.B) {
	g := NewGeneralizer(DefaultGeneralizationConfig())
	ip := "192.168.1.100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.GeneralizeIP(ip)
	}
}

func BenchmarkGeneralizeTimestamp(b *testing.B) {
	g := NewGeneralizer(DefaultGeneralizationConfig())
	ts := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.GeneralizeTimestamp(ts)
	}
}

func BenchmarkGeneralizePath(b *testing.B) {
	g := NewGeneralizer(DefaultGeneralizationConfig())
	path := "/home/user/documents/project/src/main.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.GeneralizePath(path)
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
