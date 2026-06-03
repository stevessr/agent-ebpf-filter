//go:build agentfeat_tls_capture

package app

func init() {
	enableBuildFeature(FeatureTLSCapture)
}
