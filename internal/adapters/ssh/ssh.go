// Package ssh, RouterOS yedek kanal SSH istemci taslağıdır. Faz 1'de
// gerçek SSH bağlantısı kurulmaz; sadece yapılandırma tipi
// tanımlanır. Gerçek istemci (örn. golang.org/x/crypto/ssh) Faz 3'te
// MikroTik adapter ile birlikte eklenecektir.
package ssh

// Config, SSH istemci yapılandırması.
type Config struct {
	Host          string
	Port          int
	Username      string
	UseKeyAuth    bool
	StrictHostKey bool
	TimeoutSec    int
}
