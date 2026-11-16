package verify

import (
	"strings"
	"testing"
	"time"
)

// fakeChecker, systemChecker'in gercek dosya sistemine/process'lere
// dokunmayan bir test cift (test double)'idir. Her alan nil birakilirsa
// guvenli bir varsayilana duser (exists/executable varsayilan true --
// "yol sorunu yok" -- running/succeeds varsayilan false).
type fakeChecker struct {
	existsFn     func(path string) bool
	executableFn func(path string) bool
	runningFn    func(pattern string) bool
	succeedsFn   func(name string, args ...string) bool

	runningCalls  int
	succeedsCalls int
}

func (f *fakeChecker) pathExists(path string) bool {
	if f.existsFn == nil {
		return true
	}
	return f.existsFn(path)
}

func (f *fakeChecker) pathExecutable(path string) bool {
	if f.executableFn == nil {
		return true
	}
	return f.executableFn(path)
}

func (f *fakeChecker) processRunning(pattern string) bool {
	f.runningCalls++
	if f.runningFn == nil {
		return false
	}
	return f.runningFn(pattern)
}

func (f *fakeChecker) commandSucceeds(name string, args ...string) bool {
	f.succeedsCalls++
	if f.succeedsFn == nil {
		return false
	}
	return f.succeedsFn(name, args...)
}

func withFakeSys(t *testing.T, f *fakeChecker) {
	t.Helper()
	old := sys
	sys = f
	t.Cleanup(func() { sys = old })
}

// ============================================================
// retryUntil -- MERKEZI retry/timeout mantigi (kucuk surelerle, HIZLI)
// ============================================================

func TestRetryUntil_SucceedsImmediately(t *testing.T) {
	ok, waited := retryUntil(time.Hour, time.Millisecond, func() bool { return true })
	if !ok {
		t.Fatal("ok=false, beklenen true")
	}
	if waited != 0 {
		t.Errorf("waited = %v, beklenen 0 (ilk denemede basarili)", waited)
	}
}

func TestRetryUntil_SucceedsAfterRetries(t *testing.T) {
	calls := 0
	ok, waited := retryUntil(100*time.Millisecond, time.Millisecond, func() bool {
		calls++
		return calls >= 3
	})
	if !ok {
		t.Fatal("ok=false, beklenen true (3. denemede basarili olmaliydi)")
	}
	if calls != 3 {
		t.Errorf("calls = %d, beklenen 3", calls)
	}
	if waited < 2*time.Millisecond {
		t.Errorf("waited = %v, en az 2 interval beklenirdi", waited)
	}
}

func TestRetryUntil_TimesOut(t *testing.T) {
	ok, waited := retryUntil(5*time.Millisecond, time.Millisecond, func() bool { return false })
	if ok {
		t.Fatal("ok=true, beklenen false (check hicbir zaman basarili olmuyor)")
	}
	if waited < 5*time.Millisecond {
		t.Errorf("waited = %v, en az maxWait kadar beklenirdi", waited)
	}
}

// ============================================================
// WaitForProcess
// ============================================================

func TestWaitForProcess_PathMissing_FailsImmediatelyWithoutRetry(t *testing.T) {
	f := &fakeChecker{existsFn: func(path string) bool { return false }}
	withFakeSys(t, f)

	start := time.Now()
	// maxWait kasitli olarak COK BUYUK -- eger kod yanlislikla retry
	// dongusune girerse bu test gercekten saatlerce surer; path-missing
	// kisa-devre YOKSA bu test TIMEOUT ile basarisiz olur.
	err := WaitForProcess("Test", "/opt/yok/boyle-bir-yol", time.Hour, time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("hata beklenirdi (yol yok), nil dondu")
	}
	if !strings.Contains(err.Error(), "YOL BULUNAMADI") {
		t.Errorf("hata mesaji 'YOL BULUNAMADI' icermeli, bulunan: %v", err)
	}
	if f.runningCalls != 0 {
		t.Errorf("yol yokken processRunning HIC cagrilmamali, cagrildi: %d kez", f.runningCalls)
	}
	if elapsed > time.Second {
		t.Errorf("yol-yok kisa-devresi ANINDA donmeli, surdu: %v", elapsed)
	}
}

func TestWaitForProcess_PathExistsAndProcessRunning_ReturnsNilImmediately(t *testing.T) {
	f := &fakeChecker{
		existsFn:  func(path string) bool { return true },
		runningFn: func(pattern string) bool { return true },
	}
	withFakeSys(t, f)

	if err := WaitForProcess("Test", "/opt/var/comment", time.Hour, time.Millisecond); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if f.runningCalls != 1 {
		t.Errorf("runningCalls = %d, beklenen 1 (ilk denemede basarili)", f.runningCalls)
	}
}

func TestWaitForProcess_NonPathPattern_SkipsExistsCheck(t *testing.T) {
	// "3DNotification" gibi / ile BASLAMAYAN pattern'ler icin yol
	// kontrolu hic yapilmamali -- existsFn cagrilmadan dogrudan
	// processRunning'e gecilmeli.
	existsCalled := false
	f := &fakeChecker{
		existsFn:  func(path string) bool { existsCalled = true; return false },
		runningFn: func(pattern string) bool { return true },
	}
	withFakeSys(t, f)

	if err := WaitForProcess("Notification", "3DNotification", time.Hour, time.Millisecond); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if existsCalled {
		t.Error("/ ile baslamayan pattern icin pathExists HIC cagrilmamali")
	}
}

func TestWaitForProcess_NeverRunning_TimesOutWithError(t *testing.T) {
	f := &fakeChecker{
		existsFn:  func(path string) bool { return true },
		runningFn: func(pattern string) bool { return false },
	}
	withFakeSys(t, f)

	err := WaitForProcess("Test", "/opt/var/comment", 5*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("hata beklenirdi (process hicbir zaman calismiyor)")
	}
	if !strings.Contains(err.Error(), "CALISMIYOR") {
		t.Errorf("hata mesaji 'CALISMIYOR' icermeli, bulunan: %v", err)
	}
}

// ============================================================
// AppTier / Space / FCS / FCSRemote -- aggregation + hizli-basarisizlik
// ============================================================

func TestAppTier_AllRunning_ReturnsNil(t *testing.T) {
	f := &fakeChecker{
		existsFn:  func(path string) bool { return true },
		runningFn: func(pattern string) bool { return true },
	}
	withFakeSys(t, f)

	if err := AppTier("/c", "/d", "/p", "/f", "/s"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
}

func TestAppTier_OneMissingPath_ReturnsErrorFast(t *testing.T) {
	// 120s'lik sabit maxWait'e ragmen, eksik-yol kisa-devresi sayesinde
	// bu test SANIYELER DEGIL, milisaniyeler icinde bitmeli.
	f := &fakeChecker{
		existsFn:  func(path string) bool { return path != "/dashboard" },
		runningFn: func(pattern string) bool { return true },
	}
	withFakeSys(t, f)

	start := time.Now()
	err := AppTier("/comment", "/dashboard", "/passport", "/fedsearch", "/swym")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("hata beklenirdi (dashboard yolu eksik)")
	}
	if !strings.Contains(err.Error(), "Dashboard") {
		t.Errorf("hata 'Dashboard' etiketini icermeli, bulunan: %v", err)
	}
	if elapsed > time.Second {
		t.Errorf("eksik-yol kisa-devresi sayesinde HIZLI donmeli, surdu: %v", elapsed)
	}
}

func TestAppTier_MultipleMissingPaths_ReturnsLastFailure(t *testing.T) {
	// Orijinal davranis (bash + ilk Go portu) KORUNDU: 'failed' her
	// basarisizlikta ÜZERİNE YAZILIR -- yani ilk DEGIL, SONUNCU hata
	// donuyor. Bu test, bu davranisi bilerek/farkinda sekilde sabitler.
	f := &fakeChecker{
		existsFn:  func(path string) bool { return path != "/dashboard" && path != "/swym" },
		runningFn: func(pattern string) bool { return true },
	}
	withFakeSys(t, f)

	err := AppTier("/comment", "/dashboard", "/passport", "/fedsearch", "/swym")
	if err == nil {
		t.Fatal("hata beklenirdi")
	}
	if !strings.Contains(err.Error(), "Swym") {
		t.Errorf("SONUNCU basarisizlik (Swym) donmeli, bulunan: %v", err)
	}
}

func TestSpace_AllRunning_ReturnsNil(t *testing.T) {
	f := &fakeChecker{
		existsFn:  func(path string) bool { return true },
		runningFn: func(pattern string) bool { return true },
	}
	withFakeSys(t, f)

	if err := Space("/space", "/internal"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
}

func TestFCS_PathMissing_ReturnsErrorFast(t *testing.T) {
	f := &fakeChecker{existsFn: func(path string) bool { return false }}
	withFakeSys(t, f)

	start := time.Now()
	err := FCS("/opt/fcs")
	if err == nil {
		t.Fatal("hata beklenirdi")
	}
	if time.Since(start) > time.Second {
		t.Error("eksik-yol kisa-devresi sayesinde HIZLI donmeli")
	}
}

func TestFCSRemote_AllRunning_ReturnsNil(t *testing.T) {
	f := &fakeChecker{
		existsFn:  func(path string) bool { return true },
		runningFn: func(pattern string) bool { return true },
	}
	withFakeSys(t, f)

	if err := FCSRemote("/opt/fcsremote"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
}

// ============================================================
// Index
// ============================================================

func TestIndex_NotExecutable_ReturnsErrorWithoutCallingStatus(t *testing.T) {
	f := &fakeChecker{executableFn: func(path string) bool { return false }}
	withFakeSys(t, f)

	err := Index("/opt/cvinit.sh")
	if err == nil {
		t.Fatal("hata beklenirdi")
	}
	if !strings.Contains(err.Error(), "bulunamadi") {
		t.Errorf("hata mesaji 'bulunamadi' icermeli, bulunan: %v", err)
	}
	if f.succeedsCalls != 0 {
		t.Errorf("calistirilabilir degilken 'status' HIC denenmemeli, denendi: %d kez", f.succeedsCalls)
	}
}

func TestIndex_ExecutableAndStatusSucceeds_ReturnsNil(t *testing.T) {
	f := &fakeChecker{
		executableFn: func(path string) bool { return true },
		succeedsFn:   func(name string, args ...string) bool { return true },
	}
	withFakeSys(t, f)

	if err := Index("/opt/cvinit.sh"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
}

// ============================================================
// LB
//
// NOT: LB'nin ZAMAN AŞIMI (hicbir servis aktif degil) kolu KASITLI
// OLARAK burada test EDILMIYOR -- LB() icindeki maxWait SABIT 60
// saniye (WaitForProcess'in aksine parametre olarak disaridan
// verilmiyor) ve pre-check (pathExists gibi) kisa-devresi yok, yani
// bu kolu dogrudan tetiklemek GERCEK 60 saniye beklenmesini
// gerektirirdi. retryUntil'in zaman-asimi mantigi zaten yukarida
// (TestRetryUntil_TimesOut) kucuk surelerle dogrulandi -- LB() sadece
// o merkezi mekanizmayi cagiriyor, burada SADECE "dogru cagiriyor mu"
// (basarili yollar) test ediliyor.
// ============================================================

func TestLB_DefaultCandidates_FirstActiveWins(t *testing.T) {
	f := &fakeChecker{
		succeedsFn: func(name string, args ...string) bool {
			// "haproxy" (ilk aday) icin systemctl is-active basarili.
			for _, a := range args {
				if a == "haproxy" {
					return true
				}
			}
			return false
		},
	}
	withFakeSys(t, f)

	if err := LB(""); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
}

func TestLB_Override_ActiveService_ReturnsNil(t *testing.T) {
	f := &fakeChecker{
		succeedsFn: func(name string, args ...string) bool {
			for _, a := range args {
				if a == "custom-lb" {
					return true
				}
			}
			return false
		},
	}
	withFakeSys(t, f)

	if err := LB("custom-lb"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
}
