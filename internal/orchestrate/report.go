package orchestrate

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// StepResult, Sequence() icinde calistirilan TEK bir adimin sonucunu
// tutar -- saglik raporunun en kucuk birimi.
type StepResult struct {
	Name     string
	Duration time.Duration
	Err      error
}

// Report, bir orchestrate.Sequence() calismasinin TAMAMININ ozetidir.
// Sequence basarili ya da basarisiz BITSIN (ilk hatada erken return
// etse bile), o ana kadar calismis adimlarin ozeti HER ZAMAN uretilir
// -- bkz. Sequence()'teki defer. Boylece "hangi adimda, ne kadar
// surede, neden durdu" sorusu islemin SONUNDA degil, o calismanin
// KENDI loglarinda/saglik raporunda hemen cevaplanabilir.
type Report struct {
	Steps       []StepResult
	StartedAt   time.Time
	EndedAt     time.Time
	ClearACL    bool
	With3DIndex bool
}

func (r *Report) record(name string, duration time.Duration, err error) {
	r.Steps = append(r.Steps, StepResult{Name: name, Duration: duration, Err: err})
}

// AnyFailed, raporlanan adimlardan en az biri basarisiz olduysa true
// doner.
func (r Report) AnyFailed() bool {
	for _, s := range r.Steps {
		if s.Err != nil {
			return true
		}
	}
	return false
}

// TotalDuration, StartedAt/EndedAt ikisi de doluysa (normal durum)
// toplam gecen sureyi doner; aksi halde 0.
func (r Report) TotalDuration() time.Duration {
	if r.StartedAt.IsZero() || r.EndedAt.IsZero() {
		return 0
	}
	return r.EndedAt.Sub(r.StartedAt)
}

// String, hem konsola basilan hem de SyncResult/health_report_*.txt
// dosyasina yazilan insan-okunabilir ozeti uretir. Saf bir bicimleme
// fonksiyonudur -- hicbir dosya/ag islemi yapmaz, bu yuzden birim
// testiyle dogrudan (gercek bir Sequence() calismasi olmadan)
// dogrulanabilir.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintln(&b, "============================================================")
	fmt.Fprintln(&b, " 3DSync Saglik Raporu")
	fmt.Fprintln(&b, "============================================================")
	if !r.StartedAt.IsZero() {
		fmt.Fprintf(&b, "Baslangic  : %s\n", r.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if !r.EndedAt.IsZero() {
		fmt.Fprintf(&b, "Bitis      : %s\n", r.EndedAt.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&b, "Toplam sure: %s\n", r.TotalDuration().Round(100*time.Millisecond))
	}
	fmt.Fprintf(&b, "Bayraklar  : --clear-acl=%v --with-3dindex=%v\n", r.ClearACL, r.With3DIndex)
	fmt.Fprintln(&b, "------------------------------------------------------------")

	if len(r.Steps) == 0 {
		fmt.Fprintln(&b, "(hicbir adim calistirilmadi)")
	}
	for i, s := range r.Steps {
		status := "OK  "
		if s.Err != nil {
			status = "HATA"
		}
		fmt.Fprintf(&b, "%2d. [%s] %-28s (%s)\n", i+1, status, s.Name, s.Duration.Round(100*time.Millisecond))
		if s.Err != nil {
			fmt.Fprintf(&b, "       -> %s\n", s.Err)
		}
	}

	fmt.Fprintln(&b, "------------------------------------------------------------")
	switch {
	case r.AnyFailed():
		fmt.Fprintln(&b, "SONUC: BASARISIZ -- yukaridaki HATA satir(lar)ina bakin.")
	case len(r.Steps) > 0:
		fmt.Fprintln(&b, "SONUC: BASARILI -- tum adimlar basarili.")
	default:
		fmt.Fprintln(&b, "SONUC: belirsiz -- hicbir adim calisamadan durdu.")
	}
	return b.String()
}

// writeReportFile, raporu diger adimlarin (fcssync, configgen, ...)
// kendi ciktilarini yazdigi AYNI konvansiyonla -- $mainpath/SyncResult/
// altina, dosya adinda zaman damgasiyla -- kaydeder. mainPath bos ise
// (env.sh henuz kaynaklanmamis/test ortami) hicbir sey yazmadan hata
// doner; cagiran taraf bunu UYARI olarak basar, Sequence()'in kendisini
// BASARISIZ yapmaz (saglik raporu yazilamamasi, senkronizasyonun
// kendisinin basarisiz oldugu anlamina gelmez).
func writeReportFile(mainPath string, r Report) (string, error) {
	if mainPath == "" {
		return "", fmt.Errorf("mainpath bos -- saglik raporu dosyaya yazilamiyor")
	}
	dir := mainPath + "/SyncResult"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("'%s' olusturulamadi: %w", dir, err)
	}
	ts := r.StartedAt
	if ts.IsZero() {
		ts = time.Now()
	}
	path := fmt.Sprintf("%s/health_report_%s.txt", dir, ts.Format("20060102_150405"))
	if err := os.WriteFile(path, []byte(r.String()), 0o644); err != nil {
		return "", fmt.Errorf("'%s' yazilamadi: %w", path, err)
	}
	return path, nil
}
