package vergeos

// Certificate represents a VergeOS SSL/TLS certificate.
type Certificate struct {
	// Key is the unique identifier for the certificate.
	Key FlexInt `json:"$key,omitempty"`
	// Domain is the primary domain for this certificate (read-only).
	Domain string `json:"domain,omitempty"`
	// DomainList is a comma-separated list of additional domains (SANs).
	DomainList string `json:"domainlist,omitempty"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Type is the certificate type: manual, letsencrypt, self_signed.
	Type string `json:"type,omitempty"`
	// Public is the public certificate in PEM format.
	Public string `json:"public,omitempty"`
	// Private is the private key in PEM format (hidden in responses).
	Private string `json:"private,omitempty"`
	// Chain is the certificate chain in PEM format.
	Chain string `json:"chain,omitempty"`
	// ACMEServer is the ACME server URL for Let's Encrypt certificates.
	ACMEServer string `json:"acme_server,omitempty"`
	// EABKid is the Key Identifier for External Account Binding (ACME).
	EABKid string `json:"eab_kid,omitempty"`
	// EABHMACKey is the HMAC key for External Account Binding (ACME).
	EABHMACKey string `json:"eab_hmac_key,omitempty"`
	// KeyType is the key type: ecdsa or rsa.
	KeyType string `json:"key_type,omitempty"`
	// RSAKeySize is the RSA key size (default: 2048).
	RSAKeySize string `json:"rsa_key_size,omitempty"`
	// Contact is the contact user ID for Let's Encrypt.
	Contact FlexInt `json:"contact,omitempty"`
	// AgreeTOS indicates agreement to terms of service.
	AgreeTOS bool `json:"agree_tos,omitempty"`
	// Valid indicates if the certificate is currently valid (read-only).
	Valid bool `json:"valid,omitempty"`
	// AutoCreated indicates if the certificate was auto-created (read-only).
	AutoCreated bool `json:"autocreated,omitempty"`
	// Expires is the certificate expiration timestamp (Unix epoch, read-only).
	Expires int64 `json:"expires,omitempty"`
	// Created is the creation timestamp (Unix epoch, read-only).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modification timestamp (Unix epoch, read-only).
	Modified int64 `json:"modified,omitempty"`
}

// CertificateCreateRequest is the request body for creating a certificate.
type CertificateCreateRequest struct {
	// DomainName is the primary domain for the certificate (required for manual/self-signed).
	DomainName string `json:"domainname,omitempty"`
	// DomainList is a comma-separated list of additional domains (SANs).
	DomainList string `json:"domainlist,omitempty"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Type is the certificate type: manual, letsencrypt, self_signed.
	Type string `json:"type,omitempty"`
	// Public is the public certificate in PEM format (required for manual).
	Public string `json:"public,omitempty"`
	// Private is the private key in PEM format (required for manual).
	Private string `json:"private,omitempty"`
	// Chain is the certificate chain in PEM format.
	Chain string `json:"chain,omitempty"`
	// ACMEServer is the ACME server URL (for letsencrypt type).
	ACMEServer string `json:"acme_server,omitempty"`
	// EABKid is the Key Identifier for External Account Binding.
	EABKid string `json:"eab_kid,omitempty"`
	// EABHMACKey is the HMAC key for External Account Binding.
	EABHMACKey string `json:"eab_hmac_key,omitempty"`
	// KeyType is the key type: ecdsa or rsa.
	KeyType string `json:"key_type,omitempty"`
	// RSAKeySize is the RSA key size.
	RSAKeySize string `json:"rsa_key_size,omitempty"`
	// Contact is the contact user ID (for letsencrypt type).
	Contact *int `json:"contact,omitempty"`
	// AgreeTOS indicates agreement to Let's Encrypt terms of service.
	AgreeTOS *bool `json:"agree_tos,omitempty"`
	// Renew forces renewal of an existing certificate.
	Renew *bool `json:"renew,omitempty"`
}

// CertificateUpdateRequest is the request body for updating a certificate.
type CertificateUpdateRequest struct {
	// DomainList is a comma-separated list of additional domains.
	DomainList *string `json:"domainlist,omitempty"`
	// Description is the certificate description.
	Description *string `json:"description,omitempty"`
	// Public is the public certificate in PEM format.
	Public *string `json:"public,omitempty"`
	// Private is the private key in PEM format.
	Private *string `json:"private,omitempty"`
	// Chain is the certificate chain in PEM format.
	Chain *string `json:"chain,omitempty"`
	// ACMEServer is the ACME server URL.
	ACMEServer *string `json:"acme_server,omitempty"`
	// EABKid is the Key Identifier for External Account Binding.
	EABKid *string `json:"eab_kid,omitempty"`
	// EABHMACKey is the HMAC key for External Account Binding.
	EABHMACKey *string `json:"eab_hmac_key,omitempty"`
	// KeyType is the key type.
	KeyType *string `json:"key_type,omitempty"`
	// RSAKeySize is the RSA key size.
	RSAKeySize *string `json:"rsa_key_size,omitempty"`
	// Contact is the contact user ID.
	Contact *int `json:"contact,omitempty"`
	// AgreeTOS indicates agreement to terms of service.
	AgreeTOS *bool `json:"agree_tos,omitempty"`
	// Renew forces renewal of the certificate.
	Renew *bool `json:"renew,omitempty"`
}

// Certificate type constants
const (
	// CertificateTypeManual indicates a manually uploaded certificate.
	CertificateTypeManual = "manual"
	// CertificateTypeLetsEncrypt indicates a Let's Encrypt (ACME) certificate.
	CertificateTypeLetsEncrypt = "letsencrypt"
	// CertificateTypeSelfSigned indicates a self-signed certificate.
	CertificateTypeSelfSigned = "self_signed"
)

// Certificate key type constants
const (
	// CertificateKeyTypeECDSA indicates an ECDSA key.
	CertificateKeyTypeECDSA = "ecdsa"
	// CertificateKeyTypeRSA indicates an RSA key.
	CertificateKeyTypeRSA = "rsa"
)

// certificateListFields are the fields to request when listing certificates.
const certificateListFields = "$key,domain,domainlist,description,type,acme_server,key_type,rsa_key_size,contact,agree_tos,valid,autocreated,expires,created,modified"

// certificateGetFields are the fields to request when getting a single certificate.
// Note: public, private, chain are excluded by default for security.
const certificateGetFields = certificateListFields
