package mikrotik

import (
	"crypto/tls"
	"crypto/x509"
	"strings"
)

// BuildAPITLSConfig, verilen Config alanlarına göre RouterOS API-SSL için
// kullanılacak *tls.Config'i üretir. Faz 7'de eklenmiştir.
//
// Davranış:
//
//   - VerifyTLS=false  → InsecureSkipVerify=true. CA / ServerName alanları
//     yorumlanmaz; geriye uyumluluk için Faz 3 davranışı korunur.
//   - VerifyTLS=true   → InsecureSkipVerify=false. CACertificatePEM verildiyse
//     RootCAs olarak kullanılır; PEM çözülemezse fail-closed (ErrInvalidCA).
//   - ServerNameOverride doluysa SNI ve peer-name doğrulamada kullanılır.
//   - MinVersion her durumda TLS1.2.
func BuildAPITLSConfig(cfg Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if !cfg.VerifyTLS {
		tlsCfg.InsecureSkipVerify = true
		// SNI override yine de set edilebilir; bazı sertifikalar için
		// handshake hostname seçimini etkiler. Doğrulama yapılmaz ama
		// override tutarlılık için kullanılır.
		if name := strings.TrimSpace(cfg.ServerNameOverride); name != "" {
			tlsCfg.ServerName = name
		}
		return tlsCfg, nil
	}

	// VerifyTLS=true.
	tlsCfg.InsecureSkipVerify = false

	if pem := strings.TrimSpace(cfg.CACertificatePEM); pem != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(pem)) {
			return nil, ErrInvalidCA
		}
		tlsCfg.RootCAs = pool
	}
	if name := strings.TrimSpace(cfg.ServerNameOverride); name != "" {
		tlsCfg.ServerName = name
	}
	return tlsCfg, nil
}
