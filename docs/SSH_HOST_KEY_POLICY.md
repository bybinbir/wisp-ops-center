# SSH Host Key Politikası (Faz 5 TOFU foundation)

`internal/adapters/ssh/hostkey.go` üç politika destekler:

| Policy | Davranış |
|---|---|
| `insecure_ignore` | Host key doğrulaması yapılmaz. Faz 4 default. Üretimde **önerilmez**. |
| `trust_on_first_use` | İlk bağlantıda fingerprint `ssh_known_hosts` tablosuna yazılır; sonraki bağlantılarda eşleşme zorunlu. Mismatch `ErrFingerprintMismatch` döner. |
| `pinned` | Credential profile'daki `ssh_host_key_fingerprint` alanı zorunlu eşleşme. Mismatch reddedilir. |

## Tablo (000005)

```
ssh_known_hosts (
  host        TEXT PK,
  fingerprint TEXT NOT NULL,
  seen_first  TIMESTAMPTZ,
  seen_last   TIMESTAMPTZ,
  notes       TEXT
)
```

## Hata Sözleşmesi

- `ErrFingerprintMismatch` — UI/audit için sınıflandırılmış sentinel; mesaj **ham fingerprint içermez**.
- `ErrPinnedMissing` — `pinned` politikası seçildi ama profilde fingerprint yok.
- `ErrUnknownPolicy` — bilinmeyen policy değeri.

## Faz 5 Enforcement Sınırı

`EnforcePolicy` Faz 5'te tek-atımlık SSH oturumlarında çağrılmaya hazırdır. RouterOS SSH adapter'ının her bağlantıdan önce çağrı yapması Faz 5'in **kalan boş slot'u** olarak işaretlendi (Faz 5 sonu eklenecek). Mevcut SSH yolu hâlâ `InsecureIgnoreHostKey` kullanır; politik enforcement testlerle kanıtlandı, runtime entegrasyonu Faz 5'in son sprint maddesi.
