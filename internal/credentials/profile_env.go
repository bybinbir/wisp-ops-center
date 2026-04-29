package credentials

// Faz R4 — env tabanlı cihaz sınıfı kimlik profilleri.
//
// WISP-R4-DUDE-TO-POP-OPS-FINISH onayıyla kabul edilen şema:
// secret'lar ayrı USERNAME/PASSWORD env değişkenlerinde dururlar
// (operator-side .env dosyasını gözle gözden geçirmek kolay,
// parser tek bir env stringinde compound format çözmez, hata
// yüzeyi küçük).
//
//   WISP_MIKROTIK_ROUTER_USERNAME / WISP_MIKROTIK_ROUTER_PASSWORD
//   WISP_MIKROTIK_AP_USERNAME     / WISP_MIKROTIK_AP_PASSWORD
//   WISP_MIMOSA_A_USERNAME        / WISP_MIMOSA_A_PASSWORD
//   WISP_MIMOSA_B_USERNAME        / WISP_MIMOSA_B_PASSWORD
//
// SNMP opsiyonel (cihaz tarafında hangi sürüm açıksa):
//
//   WISP_SNMP_V2_COMMUNITY
//   WISP_SNMP_V3_USERNAME
//   WISP_SNMP_V3_AUTH_PASSWORD
//   WISP_SNMP_V3_PRIV_PASSWORD
//
// Sözleşme:
//   • Hiçbir secret bu paketten log/audit/HTTP yanıtı/hata mesajı
//     üzerinden dışarı sızmaz; Sanitize() ile maskelenir.
//   • USERNAME boş ama PASSWORD set ise (ya da tersi) yükleme
//     reddedilir — operatör konfigürasyonu yarım girmiş, sessiz
//     devam etmek 928 sahte credential_failed kaydı yaratır.
//   • Bu paket cihaza bağlanmaz, sadece env okur ve tipli profile
//     yapısına çevirir.

import (
	"errors"
	"fmt"
	"os"
)

// Sınıf bazlı env anahtar sabitleri. Tek bir yerde tutulduğu için
// rename / yeniden numaralandırma test edilebilir kalır.
const (
	EnvMikrotikRouterUsername = "WISP_MIKROTIK_ROUTER_USERNAME"
	EnvMikrotikRouterPassword = "WISP_MIKROTIK_ROUTER_PASSWORD"

	EnvMikrotikAPUsername = "WISP_MIKROTIK_AP_USERNAME"
	EnvMikrotikAPPassword = "WISP_MIKROTIK_AP_PASSWORD"

	EnvMimosaAUsername = "WISP_MIMOSA_A_USERNAME"
	EnvMimosaAPassword = "WISP_MIMOSA_A_PASSWORD"

	EnvMimosaBUsername = "WISP_MIMOSA_B_USERNAME"
	EnvMimosaBPassword = "WISP_MIMOSA_B_PASSWORD"

	EnvSNMPv2Community    = "WISP_SNMP_V2_COMMUNITY"
	EnvSNMPv3Username     = "WISP_SNMP_V3_USERNAME"
	EnvSNMPv3AuthPassword = "WISP_SNMP_V3_AUTH_PASSWORD"
	EnvSNMPv3PrivPassword = "WISP_SNMP_V3_PRIV_PASSWORD"
)

// SNMPProfile, SNMP tarafının sırlarını taşır. Cihaz tarafında ya
// v2c (sadece community) ya da v3 (kullanıcı + auth + priv)
// yapılandırılır; her ikisi de tanımlıysa probe katmanı cihazın
// konuştuğu sürümü seçer.
type SNMPProfile struct {
	V2Community  string
	V3Username   string
	V3AuthSecret string
	V3PrivSecret string
}

// HasV2 SNMP v2c community değerinin set edilip edilmediğini söyler.
func (s SNMPProfile) HasV2() bool { return s.V2Community != "" }

// HasV3 SNMP v3 (kullanıcı + auth) profilinin tam olup olmadığını
// söyler; priv seçenektir.
func (s SNMPProfile) HasV3() bool { return s.V3Username != "" && s.V3AuthSecret != "" }

// Sanitized tüm sırları maskeleyen bir kopya döner.
func (s SNMPProfile) Sanitized() SNMPProfile {
	if s.V2Community != "" {
		s.V2Community = "***"
	}
	if s.V3AuthSecret != "" {
		s.V3AuthSecret = "***"
	}
	if s.V3PrivSecret != "" {
		s.V3PrivSecret = "***"
	}
	return s
}

// ProfileSet, R4'ün tüketebileceği bütün kimlik profillerini paketler.
// Operatör eşleşen USERNAME/PASSWORD çiftini set etmediğinde ilgili
// alan nil kalır. Probe kodu cihaz sınıfına uygun profilleri sırayla
// dener; hangi env anahtarının hangi sırrı verdiğini bilmez.
type ProfileSet struct {
	MikrotikRouter *Profile
	MikrotikAP     *Profile
	MimosaA        *Profile
	MimosaB        *Profile
	SNMP           SNMPProfile
}

// HasMikrotikRouter router profilinin set olup olmadığını söyler.
func (s ProfileSet) HasMikrotikRouter() bool {
	return s.MikrotikRouter != nil && s.MikrotikRouter.SecretSet()
}

// HasMikrotikAP AP profilinin set olup olmadığını söyler.
func (s ProfileSet) HasMikrotikAP() bool {
	return s.MikrotikAP != nil && s.MikrotikAP.SecretSet()
}

// HasMimosa en az bir Mimosa profilinin yüklenip yüklenmediğini söyler.
func (s ProfileSet) HasMimosa() bool {
	return (s.MimosaA != nil && s.MimosaA.SecretSet()) ||
		(s.MimosaB != nil && s.MimosaB.SecretSet())
}

// MimosaProfiles Mimosa profillerini fallback sırasıyla döner
// (önce A, sonra B). Hiçbiri yoksa boş slice döner.
func (s ProfileSet) MimosaProfiles() []*Profile {
	out := make([]*Profile, 0, 2)
	if s.MimosaA != nil && s.MimosaA.SecretSet() {
		out = append(out, s.MimosaA)
	}
	if s.MimosaB != nil && s.MimosaB.SecretSet() {
		out = append(out, s.MimosaB)
	}
	return out
}

// SanitizedSet bütün sırları maskelenmiş bir kopya döner. Profile
// referansı log / audit / HTTP sınırını geçtiği her noktada bunu
// kullan.
func (s ProfileSet) SanitizedSet() ProfileSet {
	cpy := s
	if s.MikrotikRouter != nil {
		san := Sanitize(*s.MikrotikRouter)
		cpy.MikrotikRouter = &san
	}
	if s.MikrotikAP != nil {
		san := Sanitize(*s.MikrotikAP)
		cpy.MikrotikAP = &san
	}
	if s.MimosaA != nil {
		san := Sanitize(*s.MimosaA)
		cpy.MimosaA = &san
	}
	if s.MimosaB != nil {
		san := Sanitize(*s.MimosaB)
		cpy.MimosaB = &san
	}
	cpy.SNMP = s.SNMP.Sanitized()
	return cpy
}

// LoadProfileSetFromEnv desteklenen tüm env çiftlerini okur. Eksik
// çiftler nil profil bırakır. Yarım girilmiş çift (sadece USERNAME
// veya sadece PASSWORD) hata döner: operatör yanlış konfigürasyonu
// probe katmanı 928 sahte credential_failed kaydı üretmeden ÖNCE
// öğrenmiş olur.
//
// Dönen hatalar env DEĞERLERİNİ ASLA yansıtmaz; sadece env anahtar
// adları görünür, böylece deployment logu sırrı sızdıramaz.
func LoadProfileSetFromEnv() (ProfileSet, error) {
	var (
		set  ProfileSet
		errs []error
	)

	if p, err := loadPair(
		"mikrotik_router", EnvMikrotikRouterUsername, EnvMikrotikRouterPassword,
		AuthRouterOSAPISSL,
	); err != nil {
		errs = append(errs, err)
	} else {
		set.MikrotikRouter = p
	}

	if p, err := loadPair(
		"mikrotik_ap", EnvMikrotikAPUsername, EnvMikrotikAPPassword,
		AuthRouterOSAPISSL,
	); err != nil {
		errs = append(errs, err)
	} else {
		set.MikrotikAP = p
	}

	if p, err := loadPair(
		"mimosa_a", EnvMimosaAUsername, EnvMimosaAPassword,
		AuthSSH,
	); err != nil {
		errs = append(errs, err)
	} else {
		set.MimosaA = p
	}

	if p, err := loadPair(
		"mimosa_b", EnvMimosaBUsername, EnvMimosaBPassword,
		AuthSSH,
	); err != nil {
		errs = append(errs, err)
	} else {
		set.MimosaB = p
	}

	set.SNMP = SNMPProfile{
		V2Community:  os.Getenv(EnvSNMPv2Community),
		V3Username:   os.Getenv(EnvSNMPv3Username),
		V3AuthSecret: os.Getenv(EnvSNMPv3AuthPassword),
		V3PrivSecret: os.Getenv(EnvSNMPv3PrivPassword),
	}
	if set.SNMP.V3Username != "" && set.SNMP.V3AuthSecret == "" {
		errs = append(errs, fmt.Errorf("%s set ama %s yok",
			EnvSNMPv3Username, EnvSNMPv3AuthPassword))
	}

	if len(errs) > 0 {
		return set, errors.Join(errs...)
	}
	return set, nil
}

// loadPair her iki env değişkeni de boşsa nil profil döner (operatör
// bu sınıfı henüz yapılandırmamış); sadece biri set ise hata döner;
// her ikisi de set ise dolu Profile döner.
func loadPair(profileID, userKey, passKey string, auth AuthType) (*Profile, error) {
	user := os.Getenv(userKey)
	pass := os.Getenv(passKey)
	if user == "" && pass == "" {
		return nil, nil
	}
	if user == "" {
		return nil, fmt.Errorf("%s set ama %s yok", passKey, userKey)
	}
	if pass == "" {
		return nil, fmt.Errorf("%s set ama %s yok", userKey, passKey)
	}
	return &Profile{
		ID:       profileID,
		Name:     profileID,
		AuthType: auth,
		Username: user,
		Secret:   pass,
	}, nil
}
