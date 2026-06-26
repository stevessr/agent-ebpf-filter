//go:build agentfeat_tls_capture

package tls

func init() {
	enableBuildFeature(FeatureTLSCapture)
}
