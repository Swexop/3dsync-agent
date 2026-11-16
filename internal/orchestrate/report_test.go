package orchestrate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReport_AnyFailed(t *testing.T) {
	t.Run("hepsi basarili", func(t *testing.T) {
		r := Report{Steps: []StepResult{{Name: "a"}, {Name: "b"}}}
		if r.AnyFailed() {
			t.Error("AnyFailed() true dondu, hicbir adimda hata yok")
		}
	})
	t.Run("biri basarisiz", func(t *testing.T) {
		r := Report{Steps: []StepResult{{Name: "a"}, {Name: "b", Err: errors.New("patladi")}}}
		if !r.AnyFailed() {
			t.Error("AnyFailed() false dondu, 'b' adiminda hata var")
		}
	})
	t.Run("hicbir adim yok", func(t *testing.T) {
		r := Report{}
		if r.AnyFailed() {
			t.Error("AnyFailed() true dondu, hic adim yokken false beklenirdi")
		}
	})
}

func TestReport_TotalDuration(t *testing.T) {
	start := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)

	t.Run("ikisi de dolu", func(t *testing.T) {
		r := Report{StartedAt: start, EndedAt: end}
		if got := r.TotalDuration(); got != 90*time.Second {
			t.Errorf("TotalDuration() = %s, beklenen 90s", got)
		}
	})
	t.Run("EndedAt bos", func(t *testing.T) {
		r := Report{StartedAt: start}
		if got := r.TotalDuration(); got != 0 {
			t.Errorf("TotalDuration() = %s, EndedAt bosken 0 beklenirdi", got)
		}
	})
}

func TestReport_String_BasariliTumAdimlar(t *testing.T) {
	start := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	r := Report{
		StartedAt: start,
		EndedAt:   start.Add(2 * time.Minute),
		Steps: []StepResult{
			{Name: "prepare-permissions", Duration: 1 * time.Second},
			{Name: "db-sync", Duration: 90 * time.Second},
		},
	}
	out := r.String()

	for _, want := range []string{"prepare-permissions", "db-sync", "SONUC: BASARILI"} {
		if !strings.Contains(out, want) {
			t.Errorf("String() ciktisinda %q bekleniyordu, bulunamadi:\n%s", want, out)
		}
	}
	if strings.Contains(out, "HATA") {
		t.Errorf("basarili bir raporda 'HATA' gecmemeli:\n%s", out)
	}
}

func TestReport_String_BasarisizAdimHemMesajiHemSonucuIcerir(t *testing.T) {
	wantErr := errors.New("ssh baglanti zaman asimina ugradi")
	r := Report{
		StartedAt: time.Now(),
		EndedAt:   time.Now(),
		Steps: []StepResult{
			{Name: "prepare-permissions", Duration: time.Second},
			{Name: "db-sync", Duration: 5 * time.Second, Err: wantErr},
		},
	}
	out := r.String()

	if !strings.Contains(out, "db-sync") || !strings.Contains(out, "HATA") {
		t.Errorf("basarisiz adim HATA olarak isaretlenmeli:\n%s", out)
	}
	if !strings.Contains(out, wantErr.Error()) {
		t.Errorf("hata mesaji raporda gorunmeli:\n%s", out)
	}
	if !strings.Contains(out, "SONUC: BASARISIZ") {
		t.Errorf("en az bir adim basarisizken SONUC: BASARISIZ beklenirdi:\n%s", out)
	}
}

func TestReport_String_HicAdimYokken(t *testing.T) {
	out := Report{}.String()
	if !strings.Contains(out, "belirsiz") {
		t.Errorf("hic adim calismamisken 'belirsiz' sonucu beklenirdi:\n%s", out)
	}
}

func TestWriteReportFile_BosMainPathHataDoner(t *testing.T) {
	if _, err := writeReportFile("", Report{}); err == nil {
		t.Error("bos mainPath ile hata beklenirdi, nil dondu")
	}
}

func TestWriteReportFile_DosyayaYazarVeIcerikDogrudur(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 11, 16, 14, 30, 0, 0, time.UTC)
	r := Report{
		StartedAt: start,
		EndedAt:   start.Add(time.Minute),
		Steps:     []StepResult{{Name: "service-stop", Duration: time.Second}},
	}

	path, err := writeReportFile(dir, r)
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	wantPath := filepath.Join(dir, "SyncResult", "health_report_20261116_143000.txt")
	if path != wantPath {
		t.Errorf("path = %q, beklenen %q", path, wantPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("yazilan dosya okunamadi: %v", err)
	}
	if !strings.Contains(string(data), "service-stop") {
		t.Errorf("dosya icerigi adim adini icermeli, icerik:\n%s", data)
	}
}
