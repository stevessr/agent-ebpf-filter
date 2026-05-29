package main

import (
	"fmt"

	"agent-ebpf-filter/internal/pathpolicy"
)

type PathClass = pathpolicy.PathClass

const (
	PathClassWorkspace       = pathpolicy.PathClassWorkspace
	PathClassSecret          = pathpolicy.PathClassSecret
	PathClassSystem          = pathpolicy.PathClassSystem
	PathClassTemp            = pathpolicy.PathClassTemp
	PathClassBuildCache      = pathpolicy.PathClassBuildCache
	PathClassCredentialStore = pathpolicy.PathClassCredentialStore
	PathClassUnknown         = pathpolicy.PathClassUnknown
)

func classifyPath(path, cwd string) PathClass {
	return pathpolicy.Classify(path, cwd, sudoUserHomeCache)
}

func pathClassTag(class PathClass) string {
	return pathpolicy.Tag(class)
}

func pathClassRisk(class PathClass) float64 {
	return pathpolicy.Risk(class)
}

func classifyBpfEventPath(event bpfEvent) PathClass {
	path := sanitizeUTF8(event.Path[:])
	if path != "" {
		return classifyPath(path, "")
	}
	extraPath := sanitizeUTF8(event.Extra4[:])
	if extraPath != "" {
		return classifyPath(extraPath, "")
	}
	return PathClassUnknown
}

func buildBpfEventPathClassSummary(event bpfEvent) string {
	class := classifyBpfEventPath(event)
	tag := pathClassTag(class)
	risk := pathClassRisk(class)
	return fmt.Sprintf("class=%s tag=%q risk=%.2f", class, tag, risk)
}
