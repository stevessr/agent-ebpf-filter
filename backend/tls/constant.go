package tls

const (
	TypePrivateKey    SensitiveDataType = "private_key"
	TypeCertificate   SensitiveDataType = "certificate"
	TypeSSHKey        SensitiveDataType = "ssh_key"
	TypeAPIKey        SensitiveDataType = "api_key"
	TypeJWTToken      SensitiveDataType = "jwt_token"
	TypePassword      SensitiveDataType = "password"
	TypeBearerToken   SensitiveDataType = "bearer_token"
	TypeAWSCredential SensitiveDataType = "aws_credential"
)