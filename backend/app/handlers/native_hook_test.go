package handlers

import "testing"

func TestNativeHookProviderTagRecognizesHarnesses(t *testing.T) {
	tests := []struct {
		name, sourceCLI, userAgent, event, want string
	}{
		{name: "dsh header", sourceCLI: "dsh", want: "DeepSeek Harness"},
		{name: "Pi header", sourceCLI: "pi", want: "Pi"},
		{name: "OMP header", sourceCLI: "omp", want: "Oh My Pi"},
		{name: "Pi user agent", userAgent: "pi-coding-agent/1.0", want: "Pi"},
		{name: "OMP user agent", userAgent: "oh-my-pi/1.0", want: "Oh My Pi"},
		{name: "legacy Gemini event", event: "BeforeTool", want: "Gemini CLI"},
		{name: "unknown", want: "Native Hook"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeHookProviderTag(test.sourceCLI, test.userAgent, test.event); got != test.want {
				t.Fatalf("nativeHookProviderTag(%q, %q, %q) = %q, want %q", test.sourceCLI, test.userAgent, test.event, got, test.want)
			}
		})
	}
}
