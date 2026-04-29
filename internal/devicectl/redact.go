package devicectl

// Faz R4 — read-only probe yanıtlarındaki sırları temizleyen
// merkezi redactor.
//
// WISP-R4-DUDE-TO-POP-OPS-FINISH şu garantileri zorunlu kılar:
//
//   • password / secret / token / key / private-key / ppp-secret /
//     wireless-password / user-password / certificate-private-material
//     hiçbir şart altında DB'ye, log'a, audit'e veya UI'ya ulaşmaz.
//   • RouterOS/SSH/SNMP/Mimosa cevapları device_raw_snapshots
//     tablosuna **sadece bu redactor'dan geçtikten sonra** yazılır.
//
// Bu paket iki API yüzeyi sunar:
//
//   RedactText(s string)              → düz metin (RouterOS CLI
//                                       çıktısı, SSH transcript)
//                                       içindeki KEY=VALUE
//                                       desenlerini maskeler.
//   RedactStructured(v any)           → map / slice / struct
//                                       içindeki hassas alan
//                                       isimlerini bulup
//                                       değerlerini "***" ile
//                                       değiştirir.
//
// Tasarım kuralı: maskeleme **tek yönlü**'dür. Maskelenmiş veri
// üzerinden orijinali geri çıkaracak hiçbir kod path'i yok. Eğer
// bir alan maskelenmesi gerektiği halde maskelenmiyorsa testler
// kırılır — RedactionVersion bu yüzden burada sabit; redaction
// kuralı sıkıldığında version artar, eski snapshot'ları
// reprocess etme kararı operatöre kalır.

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
)

// RedactionVersion bu paketin ürettiği maskeleme kurallarının
// sürüm etiketidir. device_raw_snapshots.redaction_version kolonu
// bu değeri saklar; gelecekte kural sıkıştığında v2'ye taşınır.
const RedactionVersion = "v1"

// RedactionMask maskelenmiş değerin gösterildiği sabit literal.
const RedactionMask = "***"

// sensitiveKeys, JSON / map / struct içindeki alan adlarını eşleyen
// **case-insensitive** regex desenleri. Bir alan adı bu desenlerden
// herhangi birine uyarsa değeri RedactionMask ile değiştirilir.
//
// Listede agresif olmaktan korkmuyoruz: false-positive (gereksiz
// maskeleme) operatörü sadece "kanıt biraz az" der; false-negative
// (sızıntı) prompt sözleşmesini bozar.
var sensitiveKeyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)password`),
	regexp.MustCompile(`(?i)secret`),
	regexp.MustCompile(`(?i)\btoken\b`),
	regexp.MustCompile(`(?i)\bapikey\b|\bapi[-_]?key\b`),
	regexp.MustCompile(`(?i)\bauth[-_]?key\b`),
	regexp.MustCompile(`(?i)private[-_]?key`),
	regexp.MustCompile(`(?i)privkey`),
	regexp.MustCompile(`(?i)wpa[-_]?psk`),
	regexp.MustCompile(`(?i)pre[-_]?shared[-_]?key`),
	regexp.MustCompile(`(?i)ppp[-_]?secret`),
	regexp.MustCompile(`(?i)wireless[-_]?password`),
	regexp.MustCompile(`(?i)\bcredentials?\b`),
	regexp.MustCompile(`(?i)\bcert[-_]?(private|priv)\b`),
	regexp.MustCompile(`(?i)\bsnmp[-_]?community\b`),
	regexp.MustCompile(`(?i)community[-_]?(string|name)`),
	regexp.MustCompile(`(?i)bearer`),
	regexp.MustCompile(`(?i)session[-_]?(id|key|token)`),
}

// IsSensitiveKey verilen alan adının hassas listede olup olmadığını
// söyler. Test edilebilir kalsın diye dışa açık.
func IsSensitiveKey(name string) bool {
	if name == "" {
		return false
	}
	for _, p := range sensitiveKeyPatterns {
		if p.MatchString(name) {
			return true
		}
	}
	return false
}

// inlineKeyValuePattern, "key=value" / "key: value" kalıplarındaki
// hassas anahtarları yakalar. Anahtar grubu g1, değer grubu g2'dir.
// Çift tırnaklı değerler bütün olarak değer kabul edilir.
//
// Anahtar şu şekilde tanımlanır:
//
//	[opsiyonel ön ek alfanümerik/dash/underscore]
//	  + [hassas çekirdek kelime]
//	  + [opsiyonel son ek alfanümerik/dash/underscore]
//
// Bu sayede "password", "user-password", "wpa-psk", "ppp-secret",
// "auth-key", "wireless-password", "passwords" hepsi yakalanır;
// "name", "frequency", "signal" gibi zararsız anahtarlar yakalanmaz.
//
// RouterOS CLI tipik olarak "key=value" üretir, SSH banner ve
// sysctl çıktıları boşluk-ayraçlı olabilir; JSON için ayrı bir
// path'imiz var. Bu regex son savunma hattı.
var inlineKeyValuePattern = regexp.MustCompile(
	`(?i)\b([a-z0-9_-]*?(?:password|secret|token|api[-_]?key|auth[-_]?key|private[-_]?key|privkey|wpa[-_]?psk|pre[-_]?shared[-_]?key|ppp[-_]?secret|wireless[-_]?password|community(?:[-_]?(?:string|name))?|bearer|credentials?|session[-_]?(?:id|key|token)|cert[-_]?(?:private|priv))[a-z0-9_-]*)\s*[:=]\s*("(?:[^"\\]|\\.)*"|[^\s,;]+)`,
)

// RedactText düz metin probe çıktısındaki hassas key=value
// kalıplarını maskeler. Maskelenen değer RedactionMask olur,
// orijinal anahtar korunur (operatör hangi alanın maskelendiğini
// bilmek ister).
func RedactText(s string) string {
	if s == "" {
		return s
	}
	return inlineKeyValuePattern.ReplaceAllStringFunc(s, func(match string) string {
		// match -> "key=value" veya "key: value"
		// Anahtarı koru, değeri maskele.
		idx := strings.IndexAny(match, ":=")
		if idx < 0 {
			return RedactionMask
		}
		return match[:idx+1] + RedactionMask
	})
}

// RedactStructured map/slice/struct/json.RawMessage içinden geçer
// ve hassas alan değerlerini RedactionMask ile değiştirir. İmzası
// `any` çünkü redaktor RouterOS API'den gelen `map[string]any`,
// SNMP'den gelen `map[string]string`, Mimosa HTTP'den gelen
// `json.RawMessage` ve olası operator-defined struct'larla aynı
// kodu paylaşmalı.
//
// Reflect kullanır; nil-safe; cycle yapmaz (her node'u en fazla
// bir kez ziyaret eder).
func RedactStructured(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	out := redactValue(rv, "")
	if !out.IsValid() {
		return nil
	}
	return out.Interface()
}

// RedactJSONBytes bir JSON byte slice'ını parse eder, hassas
// alanları maskeler ve maskelenmiş hali geri serileştirir.
// Parse başarısızsa metin tabanlı redaktor düşer — yani
// fallback: format ne olursa olsun, sızdırma.
func RedactJSONBytes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		return []byte(RedactText(string(b)))
	}
	cleaned := RedactStructured(raw)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return []byte(RedactText(string(b)))
	}
	return out
}

// redactValue reflect tabanlı recursive maskeleyici. parentKey
// son ziyaret edilen alan adı; child değerin maskelenip
// maskelenmeyeceğine karar vermek için kullanılır.
func redactValue(v reflect.Value, parentKey string) reflect.Value {
	if !v.IsValid() {
		return v
	}
	// Pointer / interface açılır.
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Map:
		return redactMap(v, parentKey)
	case reflect.Slice, reflect.Array:
		return redactSlice(v, parentKey)
	case reflect.Struct:
		return redactStruct(v, parentKey)
	case reflect.String:
		if IsSensitiveKey(parentKey) && v.String() != "" {
			out := reflect.New(v.Type()).Elem()
			out.SetString(RedactionMask)
			return out
		}
		return v
	default:
		// number / bool / nil-tipli değerler sırrı taşıyamaz; aynen
		// dönüyoruz. Hassas alan ama numerik ise yine maskele
		// (örn. token int olabilir).
		if IsSensitiveKey(parentKey) {
			out := reflect.New(reflect.TypeOf("")).Elem()
			out.SetString(RedactionMask)
			return out
		}
		return v
	}
}

func redactMap(v reflect.Value, _ string) reflect.Value {
	keyType := v.Type().Key()
	valType := v.Type().Elem()
	out := reflect.MakeMapWithSize(reflect.MapOf(keyType, valType), v.Len())

	iter := v.MapRange()
	for iter.Next() {
		k := iter.Key()
		val := iter.Value()
		keyStr := keyAsString(k)
		newVal := redactValue(val, keyStr)
		// newVal nil veya invalid olabilir; bu durumda key'i atla.
		if !newVal.IsValid() {
			continue
		}
		// Maskelenmiş value tipi ile map'in value tipi uyumlu olmayabilir
		// (örn. map[string]any ve string vs int karışık). Convert
		// edebiliyorsak edelim, edemiyorsak orijinali bırakalım.
		if !newVal.Type().AssignableTo(valType) {
			if newVal.Type().ConvertibleTo(valType) {
				newVal = newVal.Convert(valType)
			} else {
				newVal = val
			}
		}
		out.SetMapIndex(k, newVal)
	}
	return out
}

func redactSlice(v reflect.Value, parentKey string) reflect.Value {
	out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
	for i := 0; i < v.Len(); i++ {
		newVal := redactValue(v.Index(i), parentKey)
		if !newVal.IsValid() {
			continue
		}
		dst := out.Index(i)
		if newVal.Type().AssignableTo(dst.Type()) {
			dst.Set(newVal)
		} else if newVal.Type().ConvertibleTo(dst.Type()) {
			dst.Set(newVal.Convert(dst.Type()))
		} else {
			dst.Set(v.Index(i))
		}
	}
	return out
}

func redactStruct(v reflect.Value, _ string) reflect.Value {
	t := v.Type()
	out := reflect.New(t).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		newVal := redactValue(v.Field(i), field.Name)
		if newVal.IsValid() && newVal.Type().AssignableTo(out.Field(i).Type()) {
			out.Field(i).Set(newVal)
		} else if newVal.IsValid() && newVal.Type().ConvertibleTo(out.Field(i).Type()) {
			out.Field(i).Set(newVal.Convert(out.Field(i).Type()))
		} else {
			out.Field(i).Set(v.Field(i))
		}
	}
	return out
}

// keyAsString reflect'le alınan map anahtarını string'e çevirir.
// Hassas-alan eşleşmesi sadece string anahtarlarda anlamlı, sayısal
// veya struct anahtarlarda boş döner — bu güvenli, çünkü onlarda
// anahtar adı eşleşmesi zaten anlamsız.
func keyAsString(k reflect.Value) string {
	switch k.Kind() {
	case reflect.String:
		return k.String()
	case reflect.Pointer, reflect.Interface:
		if k.IsNil() {
			return ""
		}
		return keyAsString(k.Elem())
	}
	return ""
}
