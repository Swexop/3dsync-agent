package preflight

import (
	"strings"
	"testing"
)

// setAllRequired, requiredVars listesindeki HER degiskeni gecerli bir
// deger ile doldurur -- testlerin "temiz" bir baslangic noktasi olmasi
// icin. Her test, bozmak istedigi tek bir degiskeni t.Setenv ile
// UZERINE yazar.
func setAllRequired(t *testing.T) {
	t.Helper()
	for _, v := range requiredVars {
		t.Setenv(v.name, "deger-"+v.name)
	}
	// optionalVars'i da bos birakmayalim ki "UYARI" sinyali testleri
	// bulandirmasin -- ayri testlerde ayrica ele aliniyor.
}

func findResult(t *testing.T, results []Result, name string) Result {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("sonuc listesinde '%s' bulunamadi", name)
	return Result{}
}

func TestCheckEnv_AllFilled_NoFailures(t *testing.T) {
	setAllRequired(t)
	results := CheckEnv()
	for _, r := range results {
		if !r.OK {
			t.Errorf("hepsi dolu olmasina ragmen basarisiz sonuc: %+v", r)
		}
	}
}

func TestCheckEnv_EmptyRequiredVar_Fails(t *testing.T) {
	setAllRequired(t)
	t.Setenv("host3", "")
	results := CheckEnv()
	r := findResult(t, results, "host3")
	if r.OK {
		t.Fatalf("host3 bos oldugu halde OK donuyor: %+v", r)
	}
	if !strings.Contains(r.Detail, "BOS") {
		t.Errorf("beklenen 'BOS' mesaji, geldi: %q", r.Detail)
	}
}

func TestCheckEnv_PlaceholderValue_Fails(t *testing.T) {
	setAllRequired(t)
	t.Setenv("host1", "CHANGE_ME_host1")
	results := CheckEnv()
	r := findResult(t, results, "host1")
	if r.OK {
		t.Fatalf("host1 hala sablon degeri tasirken OK donuyor: %+v", r)
	}
	if !strings.Contains(r.Detail, "SABLON") {
		t.Errorf("beklenen 'SABLON' mesaji, geldi: %q", r.Detail)
	}
}

func TestCheckEnv_OptionalEmpty_IsWarnNotFail(t *testing.T) {
	setAllRequired(t)
	t.Setenv("host6", "")
	results := CheckEnv()
	r := findResult(t, results, "host6")
	if !r.OK || !r.Warn {
		t.Fatalf("host6 bos oldugunda OK+Warn bekleniyordu, geldi: %+v", r)
	}
}

func TestCheckEnv_OptionalPlaceholder_StillFails(t *testing.T) {
	// Opsiyonel bir degisken BOS olabilir ama hala "CHANGE_ME_" sablonu
	// tasiyorsa bu bir HATA'dir (kullanici kasten mi bos birakti yoksa
	// doldurmayi mi unuttu ayirt edilemez -- guvenli taraf HATA).
	setAllRequired(t)
	t.Setenv("host6", "CHANGE_ME_host6")
	results := CheckEnv()
	r := findResult(t, results, "host6")
	if r.OK {
		t.Fatalf("host6 sablon degeri tasirken OK donuyor: %+v", r)
	}
}

func TestAnyFailed(t *testing.T) {
	withFailure := []Result{
		{Name: "a", OK: true},
		{Name: "b", OK: true, Warn: true},
		{Name: "c", OK: false, Detail: "hata"},
	}
	if !AnyFailed(withFailure) {
		t.Fatal("en az bir basarisiz sonuc varken AnyFailed=false donmeli")
	}

	onlyOK := []Result{{Name: "a", OK: true}, {Name: "b", OK: true, Warn: true}}
	if AnyFailed(onlyOK) {
		t.Fatal("hic basarisiz sonuc yokken AnyFailed=true donmemeli")
	}
}
