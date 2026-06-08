package tls

import (
	"strings"
	"testing"
)

func TestRemoveSensitiveData_PrivateKey(t *testing.T) {
	kr := NewKeyRemover()

	testCases := []struct {
		name     string
		input    string
		contains string
		notContains string
	}{
		{
			name: "RSA private key",
			input: `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA1234567890abcdefghijklmnopqrstuvwxyz
-----END RSA PRIVATE KEY-----`,
			contains: "[PRIVATE_KEY_REMOVED]",
			notContains: "MIIEpAIBAAKCAQEA",
		},
		{
			name: "Generic private key",
			input: `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC
-----END PRIVATE KEY-----`,
			contains: "[PRIVATE_KEY_REMOVED]",
			notContains: "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC",
		},
		{
			name: "SSH private key",
			input: `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
-----END OPENSSH PRIVATE KEY-----`,
			contains: "[SSH_PRIVATE_KEY_REMOVED]",
			notContains: "b3BlbnNzaC1rZXktdjEAAAAABG5vbmU",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := kr.RemoveSensitiveString(tc.input)

			if !strings.Contains(result, tc.contains) {
				t.Errorf("Expected result to contain %q, got: %s", tc.contains, result)
			}

			if strings.Contains(result, tc.notContains) {
				t.Errorf("Result should not contain sensitive data %q, got: %s", tc.notContains, result)
			}
		})
	}
}

func TestRemoveSensitiveData_Certificate(t *testing.T) {
	kr := NewKeyRemover()

	input := `-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAKL0UG+mRkSvMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
-----END CERTIFICATE-----`

	result := kr.RemoveSensitiveString(input)

	if !strings.Contains(result, "[CERTIFICATE_REMOVED]") {
		t.Errorf("Expected certificate to be removed, got: %s", result)
	}

	if strings.Contains(result, "MIIDXTCCAkWgAwIBAgIJAKL0UG+mRkSvMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV") {
		t.Errorf("Certificate data should be removed")
	}
}

func TestRemoveSensitiveData_SSHKeys(t *testing.T) {
	kr := NewKeyRemover()

	testCases := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "SSH RSA public key",
			input:    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890 user@host",
			contains: "[SSH_RSA_KEY_REMOVED]",
		},
		{
			name:     "SSH ED25519 key",
			input:    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAbcdefghijklmnopqrstuvwxyz user@host",
			contains: "[SSH_ED25519_KEY_REMOVED]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := kr.RemoveSensitiveString(tc.input)

			if !strings.Contains(result, tc.contains) {
				t.Errorf("Expected result to contain %q, got: %s", tc.contains, result)
			}
		})
	}
}

func TestRemoveSensitiveData_AWSCredentials(t *testing.T) {
	kr := NewKeyRemover()

	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "AWS access key",
			input: "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "AWS secret key",
			input: "aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := kr.RemoveSensitiveString(tc.input)

			if strings.Contains(result, "AKIAIOSFODNN7EXAMPLE") {
				t.Errorf("AWS access key should be removed")
			}

			if strings.Contains(result, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY") {
				t.Errorf("AWS secret key should be removed")
			}
		})
	}
}

func TestRemoveSensitiveData_JWT(t *testing.T) {
	kr := NewKeyRemover()

	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

	result := kr.RemoveSensitiveString(input)

	if strings.Contains(result, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9") {
		t.Errorf("JWT token should be removed, got: %s", result)
	}

	if !strings.Contains(result, "REMOVED") {
		t.Errorf("Result should indicate removal, got: %s", result)
	}
}

func TestRemoveSensitiveData_APIKeys(t *testing.T) {
	kr := NewKeyRemover()

	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "API key with equals",
			input: "api_key=sk_test_1234567890abcdefghijklmnopqrstuvwxyz",
		},
		{
			name:  "API key with colon",
			input: "apikey: abcdef123456789012345678901234567890",
		},
		{
			name:  "Access key",
			input: "access_key=\"myaccesskey1234567890abcdef\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := kr.RemoveSensitiveString(tc.input)

			if strings.Contains(result, "sk_test_1234567890abcdefghijklmnopqrstuvwxyz") ||
				strings.Contains(result, "abcdef123456789012345678901234567890") ||
				strings.Contains(result, "myaccesskey1234567890abcdef") {
				t.Errorf("API key should be removed, got: %s", result)
			}
		})
	}
}

func TestRemoveSensitiveData_Passwords(t *testing.T) {
	kr := NewKeyRemover()

	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "Password with equals",
			input: "password=MySecretPassword123",
		},
		{
			name:  "Password with colon",
			input: "passwd: AnotherSecret456",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := kr.RemoveSensitiveString(tc.input)

			if strings.Contains(result, "MySecretPassword123") ||
				strings.Contains(result, "AnotherSecret456") {
				t.Errorf("Password should be removed, got: %s", result)
			}
		})
	}
}

func TestContainsSensitiveData(t *testing.T) {
	kr := NewKeyRemover()

	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Contains private key",
			input:    "-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----",
			expected: true,
		},
		{
			name:     "Contains SSH key",
			input:    "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC1234567890abcdefghijklmnopqrstuvwxyz",
			expected: true,
		},
		{
			name:     "Contains JWT",
			input:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc",
			expected: true,
		},
		{
			name:     "Contains AWS key",
			input:    "AKIAIOSFODNN7EXAMPLE",
			expected: true,
		},
		{
			name:     "No sensitive data",
			input:    "This is just normal text without any sensitive information.",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := kr.ContainsSensitiveData([]byte(tc.input))
			if result != tc.expected {
				t.Errorf("Expected %v, got %v for input: %s", tc.expected, result, tc.input)
			}
		})
	}
}

func TestDetectSensitiveData(t *testing.T) {
	kr := NewKeyRemover()

	input := `{
  "privateKey": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----",
  "apiKey": "sk_test_1234567890abcdefghijklmnop",
  "password": "MySecretPass123"
}`

	results := kr.DetectSensitiveData([]byte(input))

	if len(results) == 0 {
		t.Errorf("Expected to detect sensitive data, got none")
	}

	// Check that we detected multiple types
	types := make(map[SensitiveDataType]bool)
	for _, result := range results {
		types[result.Type] = true
	}

	if len(types) < 2 {
		t.Errorf("Expected to detect multiple types of sensitive data, got %d types", len(types))
	}
}

func TestKeyRemover_Disabled(t *testing.T) {
	kr := NewKeyRemover()
	kr.SetEnabled(false)

	input := "-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----"
	result := kr.RemoveSensitiveString(input)

	if result != input {
		t.Errorf("When disabled, data should not be modified. Got: %s", result)
	}

	if kr.ContainsSensitiveData([]byte(input)) {
		t.Errorf("When disabled, should not detect sensitive data")
	}
}

func TestMultipleSensitiveDataInSameInput(t *testing.T) {
	kr := NewKeyRemover()

	input := `
Config:
  api_key: sk_test_abc123def456
  password: MyPassword123
  certificate: -----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAKL0
-----END CERTIFICATE-----
  aws_key: AKIAIOSFODNN7EXAMPLE
`

	result := kr.RemoveSensitiveString(input)

	// Check that all sensitive data was removed
	if strings.Contains(result, "sk_test_abc123def456") {
		t.Errorf("API key should be removed")
	}
	if strings.Contains(result, "MyPassword123") {
		t.Errorf("Password should be removed")
	}
	if strings.Contains(result, "MIIDXTCCAkWgAwIBAgIJAKL0") {
		t.Errorf("Certificate should be removed")
	}
	if strings.Contains(result, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("AWS key should be removed")
	}

	// Check that removal indicators are present
	if !strings.Contains(result, "REMOVED") {
		t.Errorf("Result should contain removal indicators")
	}
}

func BenchmarkRemoveSensitiveData_Small(b *testing.B) {
	kr := NewKeyRemover()
	data := []byte("This is a test message with api_key=sk_test_1234567890 in it")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kr.RemoveSensitiveData(data)
	}
}

func BenchmarkRemoveSensitiveData_Large(b *testing.B) {
	kr := NewKeyRemover()
	data := []byte(strings.Repeat("Some normal text here. ", 100) +
		"api_key=sk_test_1234567890 " +
		strings.Repeat("More normal text. ", 100))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kr.RemoveSensitiveData(data)
	}
}

func BenchmarkContainsSensitiveData(b *testing.B) {
	kr := NewKeyRemover()
	data := []byte("This is a test message with api_key=sk_test_1234567890 in it")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kr.ContainsSensitiveData(data)
	}
}
