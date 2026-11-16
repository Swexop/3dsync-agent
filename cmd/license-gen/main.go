// license-gen -- SADECE KhanTech ic kullanimi icin: musteriye verilecek
// 3dsync-agent binary'sine GITMEZ (bkz. build.sh/install.sh -- hicbiri
// bu binary'yi derlemez/dagitmaz). Bu araci musteriye gonderme.
//
// Musteriye yeni bir lisans dosyasi (license.lic) uretmek ya da yeni bir
// anahtar cifti olusturmak icin kullanilir. Private key (ed25519) SADECE
// bu aracla ve SADECE bizde tutulan bir dosyada bulunur -- internal/
// license paketindeki PublicKeyHex, imzayi DOGRULAMAYA yarar, imzalamaya
// YARAMAZ (bkz. o paketin yorumu).
//
// KULLANIM:
//
//	# Bir kerelik: yeni anahtar cifti uret (SADECE anahtar rotasyonu
//	# gerektiginde -- normalde zaten bir private key dosyaniz var).
//	license-gen -genkeypair -out 3dsync-license-private.key
//	# -> ciktida basilan PUBLIC_KEY_HEX degerini internal/license/
//	#    license.go'daki PublicKeyHex const'una yapistirip yeniden
//	#    derleyin -- BU ISLEM TUM eski lisanslari GECERSIZ KILAR.
//
//	# Musteri icin lisans uret:
//	license-gen -privkey 3dsync-license-private.key \
//	  -customer "TOGG" -expires 2027-11-16 \
//	  [-host-fingerprint <hex>] \
//	  -out license.lic
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"khantech.com/3dsync-agent/internal/license"
)

func main() {
	genKeypair := flag.Bool("genkeypair", false, "yeni bir Ed25519 anahtar cifti uret (private key'i -out'a yazar, public key'i hex olarak basar)")
	privKeyPath := flag.String("privkey", "", "private key dosyasi (hex, -genkeypair ile uretilen)")
	customer := flag.String("customer", "", "musteri adi (orn. 'TOGG')")
	expires := flag.String("expires", "", "lisans bitis tarihi, YYYY-MM-DD formatinda (gun sonu, UTC)")
	hostFingerprint := flag.String("host-fingerprint", "", "opsiyonel: bu lisansi belirli bir host'a sabitler (internal/license.HostFingerprint() ciktisi). Bos birakilirsa host kontrolu YAPILMAZ.")
	out := flag.String("out", "license.lic", "cikti dosyasi yolu")
	flag.Parse()

	if *genKeypair {
		runGenKeypair(*out)
		return
	}

	runGenLicense(*privKeyPath, *customer, *expires, *hostFingerprint, *out)
}

func runGenKeypair(out string) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	dieOn(err)

	data := []byte(hex.EncodeToString(priv) + "\n")
	// 0600: bu dosya bir SIR -- sadece sahibi okuyabilsin/yazabilsin.
	dieOn(os.WriteFile(out, data, 0o600))

	fmt.Printf("Private key yazildi: %s (BUNU GUVENLI SAKLAYIN, git'e KOYMAYIN, sadece size ait bir yerde tutun)\n", out)
	fmt.Printf("PUBLIC_KEY_HEX=%s\n", hex.EncodeToString(pub))
	fmt.Println()
	fmt.Println("Yukaridaki PUBLIC_KEY_HEX degerini internal/license/license.go'daki")
	fmt.Println("PublicKeyHex const'una yapistirip 3dsync-agent'i YENIDEN DERLEYIN.")
	fmt.Println("DIKKAT: bu islem, eski anahtarla uretilmis TUM license.lic dosyalarini")
	fmt.Println("GECERSIZ KILAR -- tum musterilere yeni lisans dagitmaniz gerekir.")
}

func runGenLicense(privKeyPath, customer, expiresStr, hostFingerprint, out string) {
	if privKeyPath == "" || customer == "" || expiresStr == "" {
		fmt.Fprintln(os.Stderr, "KULLANIM: license-gen -privkey <dosya> -customer <ad> -expires YYYY-MM-DD [-host-fingerprint <hex>] [-out license.lic]")
		fmt.Fprintln(os.Stderr, "      veya: license-gen -genkeypair [-out private.key]")
		os.Exit(1)
	}

	privHexRaw, err := os.ReadFile(privKeyPath)
	dieOn(err)
	privHex := string(trimNewline(privHexRaw))
	privBytes, err := hex.DecodeString(privHex)
	dieOn(err)
	if len(privBytes) != ed25519.PrivateKeySize {
		fmt.Fprintf(os.Stderr, "HATA: '%s' gecerli bir Ed25519 private key gibi gorunmuyor (beklenen %d byte, bulunan %d byte)\n", privKeyPath, ed25519.PrivateKeySize, len(privBytes))
		os.Exit(1)
	}
	priv := ed25519.PrivateKey(privBytes)

	// Bitis tarihi, o GUNUN SONUNA (23:59:59 UTC) ayarlanir -- "2027-11-16"
	// verilip o gun hala calismasi beklenirken sabah 00:00'da suresi
	// dolmus gibi davranmamasi icin.
	expiresDate, err := time.Parse("2006-01-02", expiresStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "HATA: '-expires' icin gecersiz tarih '%s' (beklenen format: YYYY-MM-DD): %v\n", expiresStr, err)
		os.Exit(1)
	}
	expiresAt := time.Date(expiresDate.Year(), expiresDate.Month(), expiresDate.Day(), 23, 59, 59, 0, time.UTC)

	lic := license.License{
		Customer:        customer,
		IssuedAt:        time.Now().UTC(),
		ExpiresAt:       expiresAt,
		HostFingerprint: hostFingerprint,
	}

	token, err := license.Sign(priv, lic)
	dieOn(err)
	dieOn(os.WriteFile(out, []byte(token+"\n"), 0o644))

	fmt.Printf("Lisans dosyasi yazildi: %s\n", out)
	fmt.Printf("  Musteri         : %s\n", lic.Customer)
	fmt.Printf("  Bitis tarihi    : %s\n", lic.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
	if lic.HostFingerprint != "" {
		fmt.Printf("  Host-fingerprint: %s (SADECE bu host'ta calisir)\n", lic.HostFingerprint)
	} else {
		fmt.Println("  Host-fingerprint: (verilmedi -- bu lisans HERHANGI bir host'ta calisir)")
	}
	fmt.Println()
	fmt.Printf("Bu dosyayi musteriye, install.sh'in beklediği '%s/config/license.lic' konumuna yerlestirecek sekilde iletin.\n", "$mainpath")
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func dieOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "HATA:", err)
		os.Exit(1)
	}
}
