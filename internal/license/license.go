// Package license, 3dsync-agent'in musteri tarafina OFFLINE (ag erisimi
// gerektirmeyen) imzali bir lisans dosyasiyla dagitilmasini saglar.
//
// NEDEN OFFLINE: musterilerimiz genelde ag'i disariya kapali (izole)
// ortamlar kullaniyor -- binary'nin bizim bir sunucumuza "phone-home"
// yapmasi bu ortamlarda ya hic calismaz ya da guvenlik politikasina
// aykiri olur. Bu yuzden lisans tamami tamamen OFFLINE dogrulanir:
// imzali bir dosya (license.lic) + binary'ye GOMULU bir public key.
//
// IMZALAMA: Ed25519 (crypto/ed25519, ek bagimlilik gerektirmez). Private
// key SADECE bizde kalir (asla bu repoya ya da musteri paketine
// girmez) -- cmd/license-gen aracini calistirmak icin kullanilir.
// Public key, imzayi DOGRULAMAK icin yeterlidir ama YENI bir lisans
// URETMEK icin yetmez, bu yuzden kaynak kodunda acikca gomulu olmasi
// (PublicKeyHex) bir guvenlik sorunu degildir.
package license

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// rawB64Encoding, lisans token'inin payload/imza bolumlerini URL-guvenli,
// dolgusuz base64 ile kodlar -- dosyada tek satirda, '.' ayracinda
// karisma riski olmadan durabilsin diye.
var rawB64Encoding = base64.RawURLEncoding

// PublicKeyHex, imza DOGRULAMA icin kullanilan Ed25519 public key'idir
// (hex, 32 byte). Bu deger GUVENLI sekilde acik edilebilir -- sadece
// VAR OLAN bir imzayi dogrulamaya yarar, yeni lisans uretmeye yaramaz
// (bunun icin internal/license/genkeypair.go ile ayri uretilen, SADECE
// cmd/license-gen'in calistirildigi makinede/guvenli bir yerde tutulan
// private key gerekir).
//
// DIKKAT: bu deger degistirilirse, o ana kadar uretilmis TUM
// license.lic dosyalari gecersiz olur (farkli bir anahtar ciftiyle
// imzalanmislar demektir) -- SADECE anahtar sifirlama/rotasyonu
// GEREKTIGINDE degistirilmeli, ve o durumda TUM musterilere yeni
// lisans dosyasi dagitilmasi gerekir.
const PublicKeyHex = "7fd8b42bc521ac2659af4c13166d582c5b710e0a8afb0d7ef5c467f6ee8d26c2"

// License, bir license.lic dosyasinin imzalanan icerigidir.
type License struct {
	Customer        string    `json:"customer"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	HostFingerprint string    `json:"host_fingerprint,omitempty"` // bos ise host kontrolu YAPILMAZ
}

// --- Imzalama / dogrulama -------------------------------------------

// Sign, verilen License'i JSON'a cevirip private key ile imzalar ve
// dosyaya yazilacak tek-satirlik token'i doner: "<base64 json>.<base64 imza>".
// SADECE cmd/license-gen tarafindan kullanilir -- 3dsync-agent'in kendisi
// bu fonksiyonu HICBIR ZAMAN cagirmaz (musteriye giden binary'de imzalama
// yetenegi OLMAMALI, sadece dogrulama).
func Sign(priv ed25519.PrivateKey, lic License) (string, error) {
	payload, err := json.Marshal(lic)
	if err != nil {
		return "", fmt.Errorf("lisans JSON'a cevrilemedi: %w", err)
	}
	sig := ed25519.Sign(priv, payload)
	return b64(payload) + "." + b64(sig), nil
}

// Verify, bir license.lic token'inin imzasini PublicKeyHex ile dogrular
// ve gecerliyse iceki License'i doner. Imza gecersizse ya da format
// bozuksa HATA doner -- bu durum Evaluate()'e hic ulasmadan dogrudan
// StatusBlocked sayilmali (bkz. cmd/3dsync-agent main.go'daki kullanim).
func Verify(token string) (License, error) {
	pub, err := hex.DecodeString(PublicKeyHex)
	if err != nil {
		return License{}, fmt.Errorf("gomulu public key hex decode edilemedi (bug): %w", err)
	}

	parts := strings.SplitN(strings.TrimSpace(token), ".", 2)
	if len(parts) != 2 {
		return License{}, errors.New("lisans dosyasi formati bozuk (beklenen: '<payload>.<imza>')")
	}
	payload, err := unb64(parts[0])
	if err != nil {
		return License{}, fmt.Errorf("lisans payload'i decode edilemedi: %w", err)
	}
	sig, err := unb64(parts[1])
	if err != nil {
		return License{}, fmt.Errorf("lisans imzasi decode edilemedi: %w", err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pub), payload, sig) {
		return License{}, errors.New("lisans imzasi GECERSIZ -- dosya bozulmus veya tahrif edilmis olabilir")
	}

	var lic License
	if err := json.Unmarshal(payload, &lic); err != nil {
		return License{}, fmt.Errorf("imza gecerli ama lisans icerigi JSON olarak okunamadi (bug): %w", err)
	}
	return lic, nil
}

func b64(b []byte) string            { return rawB64Encoding.EncodeToString(b) }
func unb64(s string) ([]byte, error) { return rawB64Encoding.DecodeString(s) }

// --- Degerlendirme (warning/grace/blocked) ---------------------------

// Status, Evaluate()'in dondurebilecegi durumlardir.
type Status int

const (
	// StatusOK: lisans gecerli, suresi/host'u sorunsuz.
	StatusOK Status = iota
	// StatusWarning: suresi WarnBefore penceresi icinde yaklasiyor ama
	// HENUZ DOLMADI -- komut CALISIR, sadece uyari basilir.
	StatusWarning
	// StatusGrace: suresi DOLDU ama GracePeriod icinde -- komut HALA
	// CALISIR (musteriyi aniden kilitlememek icin), uyari daha SERT.
	StatusGrace
	// StatusBlocked: suresi GracePeriod'u da asti, YA DA host-fingerprint
	// uyusmuyor, YA DA imza/format gecersiz -- komut CALISMAZ.
	StatusBlocked
)

// Varsayilan esikler -- bkz. kullanici ile mutabik kalinan tasarim:
// suresi dolmadan 30 gun once uyari baslar, doldugunda 14 gunluk bir
// grace period var, ondan sonra sert blok. Host-fingerprint uyusmazligi
// bu esiklerden BAGIMSIZ, HER ZAMAN dogrudan StatusBlocked'tir (bu
// zamanlama meselesi degil, gercek bir ihlal/kopyalama durumudur).
const (
	DefaultWarnBefore  = 30 * 24 * time.Hour
	DefaultGracePeriod = 14 * 24 * time.Hour
)

// Result, Evaluate()'in insan-okunabilir sonucu.
type Result struct {
	Status  Status
	Message string
}

// Evaluate, bir License'i verilen zaman ve host-fingerprint'e karsi
// degerlendirir. Saf bir fonksiyondur -- dosya/ag islemi yapmaz, bu
// yuzden gercek bir license.lic dosyasi ya da gercek bir host olmadan
// birim testiyle dogrudan test edilebilir.
func Evaluate(lic License, now time.Time, currentFingerprint string, warnBefore, gracePeriod time.Duration) Result {
	// Host-fingerprint kontrolu HER SEYDEN ONCE: lisans belirli bir
	// host'a sabitlenmisse (HostFingerprint bos degilse) ve mevcut
	// host uyusmuyorsa, sure/grace ne olursa olsun BLOKE -- bu bir
	// zamanlama meselesi degil, lisansin baska bir makineye kopyalanmis
	// olmasi durumudur.
	if lic.HostFingerprint != "" && lic.HostFingerprint != currentFingerprint {
		return Result{
			Status: StatusBlocked,
			Message: fmt.Sprintf(
				"HATA: Bu lisans ('%s' musterisi icin) BASKA bir makineye ait "+
					"(host-fingerprint uyusmuyor). 3dsync-agent bu host'ta CALISMAYACAK. "+
					"Bu host icin yeni bir lisans dosyasi talep edin.",
				lic.Customer),
		}
	}

	switch {
	case now.Before(lic.ExpiresAt.Add(-warnBefore)):
		return Result{Status: StatusOK, Message: fmt.Sprintf("Lisans gecerli (musteri: %s, bitis: %s).", lic.Customer, lic.ExpiresAt.Format("2006-01-02"))}

	case now.Before(lic.ExpiresAt):
		daysLeft := int(lic.ExpiresAt.Sub(now).Hours()/24) + 1
		return Result{
			Status: StatusWarning,
			Message: fmt.Sprintf(
				"UYARI: '%s' lisansinin suresi %d gun icinde (%s) dolacak. "+
					"Suresi dolmadan once yenileme icin KhanTech ile iletisime gecin.",
				lic.Customer, daysLeft, lic.ExpiresAt.Format("2006-01-02")),
		}

	case now.Before(lic.ExpiresAt.Add(gracePeriod)):
		daysOverdue := int(now.Sub(lic.ExpiresAt).Hours()/24) + 1
		daysLeftInGrace := int(lic.ExpiresAt.Add(gracePeriod).Sub(now).Hours() / 24)
		return Result{
			Status: StatusGrace,
			Message: fmt.Sprintf(
				"UYARI: '%s' lisansinin suresi %d gun once (%s) DOLDU -- su an gecici "+
					"ek sure (grace period) icindesiniz, %d gun sonra 3dsync-agent "+
					"CALISMAYI DURDURACAK. Lutfen ACIL olarak lisansi yenileyin.",
				lic.Customer, daysOverdue, lic.ExpiresAt.Format("2006-01-02"), daysLeftInGrace),
		}

	default:
		return Result{
			Status: StatusBlocked,
			Message: fmt.Sprintf(
				"HATA: '%s' lisansinin suresi %s tarihinde doldu ve ek sure (grace period) de "+
					"asildi. 3dsync-agent bu lisansla artik CALISMAYACAK. Lutfen KhanTech ile "+
					"iletisime gecip lisansi yenileyin.",
				lic.Customer, lic.ExpiresAt.Format("2006-01-02")),
		}
	}
}

// --- Host-fingerprint --------------------------------------------------

// HostFingerprint, bu host'u kararli bicimde tanimlayan bir kimlik
// doner: Linux'ta /etc/machine-id dosyasinin SHA-256 ozeti (hex).
// machine-id, systemd tarafindan kurulumda/ilk boot'ta uretilir, reboot'ta
// DEGISMEZ, ve (cloud-init gibi araclarla duzgun klonlanan VM'ler haric)
// klonlamada genelde yeniden uretilir -- MAC adresine kiyasla NIC
// degisiminden/sanallastirmadan daha az etkilenir.
func HostFingerprint() (string, error) {
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return "", fmt.Errorf("/etc/machine-id okunamadi (bu host-fingerprint'in kaynagidir): %w", err)
	}
	sum := sha256.Sum256(bytes.TrimSpace(data))
	return hex.EncodeToString(sum[:]), nil
}
