package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"
)

// testKeypair, SADECE bu test dosyasinda kullanilan, PublicKeyHex ile
// HICBIR ILGISI OLMAYAN gecici bir anahtar ciftidir -- Sign()'in
// kendisini (gercek private key'den tamamen bagimsiz) dogrulamak icin.
// Verify() HER ZAMAN gomulu PublicKeyHex'e karsi dogrular, bu yuzden bu
// test ciftiyle imzalanan token'lar Verify()'dan GECMEZ (asagidaki
// TestSign_RoundTrip_WithOwnKeypairNotVerify testi tam bunu dogrular).
func testKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("test anahtar cifti uretilemedi: %v", err)
	}
	return pub, priv
}

func TestSign_ThenVerifyWithMatchingKey_RoundTrips(t *testing.T) {
	pub, priv := testKeypair(t)
	lic := License{
		Customer:  "ACME",
		IssuedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	token, err := Sign(priv, lic)
	if err != nil {
		t.Fatalf("Sign() hata: %v", err)
	}

	// Verify()'in kendi ic mantigini, gomulu PublicKeyHex'ten BAGIMSIZ
	// olarak dogrulamak icin ayni dogrulama adimlarini elle tekrarliyoruz
	// (parcalama + base64 decode + ed25519.Verify) -- boylece hem Sign
	// hem Verify'in AYNI token formatini urettigini/okudugunu, gercek
	// production public key'ine bagli olmadan test edebiliyoruz.
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("token formati beklenen '<payload>.<imza>' degil: %q", token)
	}
	payload, err := unb64(parts[0])
	if err != nil {
		t.Fatalf("payload decode edilemedi: %v", err)
	}
	sig, err := unb64(parts[1])
	if err != nil {
		t.Fatalf("imza decode edilemedi: %v", err)
	}
	if !ed25519.Verify(pub, payload, sig) {
		t.Fatal("imza, AYNI anahtar ciftiyle dogrulanamadi")
	}
}

func TestVerify_RejectsTokenSignedWithDifferentKey(t *testing.T) {
	_, priv := testKeypair(t) // production public key'iyle ESLESMEYEN bir cift
	lic := License{Customer: "ACME", ExpiresAt: time.Now().Add(365 * 24 * time.Hour)}

	token, err := Sign(priv, lic)
	if err != nil {
		t.Fatalf("Sign() hata: %v", err)
	}

	if _, err := Verify(token); err == nil {
		t.Fatal("baska bir anahtarla imzalanmis token, gomulu PublicKeyHex ile dogrulanmamaliydi")
	}
}

func TestVerify_RejectsTamperedPayload(t *testing.T) {
	_, priv := testKeypair(t)
	lic := License{Customer: "ACME", ExpiresAt: time.Now().Add(365 * 24 * time.Hour)}
	token, err := Sign(priv, lic)
	if err != nil {
		t.Fatalf("Sign() hata: %v", err)
	}

	parts := strings.SplitN(token, ".", 2)
	tampered := b64([]byte(`{"customer":"BASKA","expires_at":"2099-01-01T00:00:00Z"}`)) + "." + parts[1]
	if _, err := Verify(tampered); err == nil {
		t.Fatal("tahrif edilmis payload, dogrulanmamaliydi")
	}
}

func TestVerify_RejectsMalformedToken(t *testing.T) {
	for _, bad := range []string{"", "tek-parca-nokta-yok", "a.b.c", "!!!.###"} {
		if _, err := Verify(bad); err == nil {
			t.Errorf("bozuk token %q hatasiz kabul edildi", bad)
		}
	}
}

func TestEvaluate_HostFingerprintMismatch_AlwaysBlocksRegardlessOfExpiry(t *testing.T) {
	lic := License{
		Customer:        "ACME",
		ExpiresAt:       time.Now().Add(365 * 24 * time.Hour), // suresi COK uzun -- onemli degil
		HostFingerprint: "host-a",
	}
	res := Evaluate(lic, time.Now(), "host-b", DefaultWarnBefore, DefaultGracePeriod)
	if res.Status != StatusBlocked {
		t.Errorf("Status = %v, host-fingerprint uyusmazliginda StatusBlocked beklenirdi", res.Status)
	}
}

func TestEvaluate_HostFingerprintMatch_OK(t *testing.T) {
	lic := License{
		Customer:        "ACME",
		ExpiresAt:       time.Now().Add(365 * 24 * time.Hour),
		HostFingerprint: "host-a",
	}
	res := Evaluate(lic, time.Now(), "host-a", DefaultWarnBefore, DefaultGracePeriod)
	if res.Status != StatusOK {
		t.Errorf("Status = %v, fingerprint eslesirken StatusOK beklenirdi", res.Status)
	}
}

func TestEvaluate_HostFingerprintEmpty_SkipsHostCheck(t *testing.T) {
	lic := License{Customer: "ACME", ExpiresAt: time.Now().Add(365 * 24 * time.Hour)} // HostFingerprint bos
	res := Evaluate(lic, time.Now(), "herhangi-bir-host", DefaultWarnBefore, DefaultGracePeriod)
	if res.Status != StatusOK {
		t.Errorf("Status = %v, HostFingerprint bosken host kontrolu atlanip StatusOK beklenirdi", res.Status)
	}
}

func TestEvaluate_ExpiryThresholds(t *testing.T) {
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	warnBefore := 30 * 24 * time.Hour
	grace := 14 * 24 * time.Hour
	lic := License{Customer: "ACME", ExpiresAt: expires}

	cases := []struct {
		name string
		now  time.Time
		want Status
	}{
		{"uyari penceresinden COK once", expires.Add(-60 * 24 * time.Hour), StatusOK},
		{"uyari penceresine tam girdigi an", expires.Add(-warnBefore).Add(-time.Second), StatusOK},
		{"uyari penceresi icinde", expires.Add(-10 * 24 * time.Hour), StatusWarning},
		{"suresi tam doldugu an", expires.Add(-time.Nanosecond), StatusWarning},
		{"grace period icinde", expires.Add(5 * 24 * time.Hour), StatusGrace},
		{"grace period'un son anina yakin", expires.Add(grace).Add(-time.Second), StatusGrace},
		{"grace period asildi", expires.Add(grace).Add(time.Second), StatusBlocked},
		{"grace period'dan COK sonra", expires.Add(365 * 24 * time.Hour), StatusBlocked},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := Evaluate(lic, c.now, "", warnBefore, grace)
			if res.Status != c.want {
				t.Errorf("Status = %v, beklenen %v (now=%s)", res.Status, c.want, c.now)
			}
			if res.Message == "" {
				t.Error("Message bos olmamali")
			}
		})
	}
}

func TestHostFingerprint_StableAcrossCalls(t *testing.T) {
	// /etc/machine-id bu sandbox'ta olmayabilir -- o durumda sadece
	// HATANIN TUTARLI oldugunu dogruluyoruz (iki cagri da ayni sonucu
	// vermeli, ikisi de hata ya da ikisi de ayni deger).
	a, errA := HostFingerprint()
	b, errB := HostFingerprint()
	if (errA == nil) != (errB == nil) {
		t.Fatalf("HostFingerprint() iki cagrida farkli hata durumu dondu: %v / %v", errA, errB)
	}
	if errA == nil && a != b {
		t.Errorf("HostFingerprint() iki cagrida farkli deger dondu: %q != %q", a, b)
	}
}
