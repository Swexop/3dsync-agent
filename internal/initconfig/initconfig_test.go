package initconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleTemplate = `#!/bin/bash
# user: SSH kullanicisi aciklamasi
user=CHANGE_ME_ssh_user

# host0 aciklamasi
# ikinci satir
host0=CHANGE_ME_host0

# DB_NAME aciklamasi
DB_NAME="CHANGE_ME_spacedb"

# turetilmis -- degismeden gecmeli
SQL_SERVER="$host0"

tomeepath=CHANGE_ME_/opt/tomee                 # trailing yorum
`

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("yazilamadi: %v", err)
	}
	return p
}

func TestRun_Interactive_FillsAllPlaceholders(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	out := filepath.Join(dir, "out.sh")

	answers := "svc3ds\n10.0.0.1\nacmedb\n/opt/acme/tomee\n"
	var stdout bytes.Buffer

	err := Run(Options{
		TemplatePath: tmpl,
		OutputPath:   out,
		In:           strings.NewReader(answers),
		Out:          &stdout,
	})
	if err != nil {
		t.Fatalf("Run basarisiz: %v\nstdout:\n%s", err, stdout.String())
	}

	result, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("cikis okunamadi: %v", err)
	}
	content := string(result)

	for _, want := range []string{
		`user=svc3ds`,
		`host0=10.0.0.1`,
		`DB_NAME="acmedb"`,
		`tomeepath=/opt/acme/tomee                 # trailing yorum`,
		`SQL_SERVER="$host0"`, // turetilmis satir degismemeli
	} {
		if !strings.Contains(content, want) {
			t.Errorf("cikista bulunamadi: %q\n--- tam cikis ---\n%s", want, content)
		}
	}
	if strings.Contains(content, "CHANGE_ME_") {
		t.Errorf("hala CHANGE_ME_ iceriyor:\n%s", content)
	}
}

func TestRun_Answers_NonInteractive_Complete(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	out := filepath.Join(dir, "out.sh")
	answersFile := writeTemp(t, dir, "answers.env", `
# yorum satiri atlanmali
user=svc3ds
host0=10.0.0.1
DB_NAME="acmedb"
tomeepath=/opt/acme/tomee
`)

	err := Run(Options{
		TemplatePath:   tmpl,
		OutputPath:     out,
		AnswersPath:    answersFile,
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("Run basarisiz: %v", err)
	}

	result, _ := os.ReadFile(out)
	if strings.Contains(string(result), "CHANGE_ME_") {
		t.Errorf("hala CHANGE_ME_ iceriyor:\n%s", result)
	}
	if !strings.Contains(string(result), `DB_NAME="acmedb"`) {
		t.Errorf("DB_NAME dogru doldurulmadi:\n%s", result)
	}
}

func TestRun_NonInteractive_MissingAnswer_ErrorsAndDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	out := filepath.Join(dir, "out.sh")
	// Sadece bir kismi cevaplanmis answers dosyasi.
	answersFile := writeTemp(t, dir, "answers.env", "user=svc3ds\n")

	err := Run(Options{
		TemplatePath:   tmpl,
		OutputPath:     out,
		AnswersPath:    answersFile,
		NonInteractive: true,
	})
	if err == nil {
		t.Fatal("eksik cevaplarla hata BEKLENIYORDU, nil dondu")
	}
	if !strings.Contains(err.Error(), "host0") {
		t.Errorf("hata mesaji eksik degiskeni (host0) belirtmeli, geldi: %v", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("hata durumunda cikis dosyasi YAZILMAMALI, ama yazilmis")
	}
}

func TestRun_InteractiveEmptyAnswer_SkipsAndErrors(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	out := filepath.Join(dir, "out.sh")

	// Sadece ilk soruya cevap verilip stdin bitiyor (EOF) -- kalanlar bos.
	err := Run(Options{
		TemplatePath: tmpl,
		OutputPath:   out,
		In:           strings.NewReader("svc3ds\n"),
		Out:          &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("eksik interaktif cevaplarla hata bekleniyordu")
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("eksik cevapta cikis dosyasi yazilmamali")
	}
}

func TestRun_InPlace_CreatesBackup(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	answersFile := writeTemp(t, dir, "answers.env", `
user=svc3ds
host0=10.0.0.1
DB_NAME="acmedb"
tomeepath=/opt/acme/tomee
`)

	err := Run(Options{
		TemplatePath:   tmpl,
		AnswersPath:    answersFile,
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("Run basarisiz: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	var foundBackup bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "env.sh.bak.") {
			foundBackup = true
		}
	}
	if !foundBackup {
		t.Fatal("yerinde guncellemede .bak.<tarih> yedegi olusturulmadi")
	}

	result, _ := os.ReadFile(tmpl)
	if strings.Contains(string(result), "CHANGE_ME_") {
		t.Errorf("yerinde guncellenen dosya hala CHANGE_ME_ iceriyor:\n%s", result)
	}
}

func TestRun_NoPlaceholders_NoOp(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", `#!/bin/bash
user=already-set
host0=10.0.0.1
`)
	out := filepath.Join(dir, "out.sh")
	var stdout bytes.Buffer

	err := Run(Options{TemplatePath: tmpl, OutputPath: out, Out: &stdout})
	if err != nil {
		t.Fatalf("Run basarisiz: %v", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("doldurulacak yer tutucu yokken cikis dosyasi yazilmamali")
	}
	if !strings.Contains(stdout.String(), "kalmamis") {
		t.Errorf("bilgilendirici mesaj beklenirdi, geldi: %q", stdout.String())
	}
}

func TestRun_InvalidTemplatePath_Errors(t *testing.T) {
	err := Run(Options{TemplatePath: "/does/not/exist/env.sh"})
	if err == nil {
		t.Fatal("var olmayan sablon yolu icin hata bekleniyordu")
	}
}

func TestRun_Interactive_GeriDuzeltir(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	out := filepath.Join(dir, "out.sh")

	// user icin once "yanlis-deger" yazilir, sonra host0'da "geri" ile
	// user'a donulup "duzeltilmis" olarak degistirilir, sonra devam
	// edilip kalan sorular normal cevaplanir.
	answers := "yanlis-deger\ngeri\nduzeltilmis\n10.0.0.1\nacmedb\n/opt/acme/tomee\n"
	var stdout bytes.Buffer

	err := Run(Options{
		TemplatePath: tmpl,
		OutputPath:   out,
		In:           strings.NewReader(answers),
		Out:          &stdout,
	})
	if err != nil {
		t.Fatalf("Run basarisiz: %v\nstdout:\n%s", err, stdout.String())
	}

	result, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("cikis okunamadi: %v", err)
	}
	content := string(result)

	if !strings.Contains(content, "user=duzeltilmis") {
		t.Errorf("'geri' ile duzeltilen deger cikista yok:\n%s", content)
	}
	if strings.Contains(content, "yanlis-deger") {
		t.Errorf("'geri' ile DUZELTILMESI gereken eski deger hala cikista:\n%s", content)
	}
}

func TestRun_Interactive_BosGecilenSoruTekrarSorulur(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	out := filepath.Join(dir, "out.sh")

	// host0'i bos Enter ile gec (defer), DB_NAME ve tomeepath'i normal
	// cevapla; sonra "eksikler" turunde host0 tekrar sorulur, bu sefer
	// cevaplanir.
	answers := "svc3ds\n\nacmedb\n/opt/acme/tomee\n10.0.0.1\n"
	var stdout bytes.Buffer

	err := Run(Options{
		TemplatePath: tmpl,
		OutputPath:   out,
		In:           strings.NewReader(answers),
		Out:          &stdout,
	})
	if err != nil {
		t.Fatalf("Run basarisiz: %v\nstdout:\n%s", err, stdout.String())
	}

	result, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("cikis okunamadi: %v", err)
	}
	content := string(result)
	if !strings.Contains(content, "host0=10.0.0.1") {
		t.Errorf("bos gecilip sonra tekrar sorulan deger cikista yok:\n%s", content)
	}
	if strings.Contains(content, "CHANGE_ME_") {
		t.Errorf("hala CHANGE_ME_ iceriyor:\n%s", content)
	}
}

func TestRun_Interactive_AtlaBilerekBosBirakir(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	out := filepath.Join(dir, "out.sh")

	// tomeepath'i "atla" ile BILEREK bos birak -- bu, bos Enter'dan
	// farkli olarak dosyanin yazilmasini ENGELLEMEMELI.
	answers := "svc3ds\n10.0.0.1\nacmedb\natla\n"
	var stdout bytes.Buffer

	err := Run(Options{
		TemplatePath: tmpl,
		OutputPath:   out,
		In:           strings.NewReader(answers),
		Out:          &stdout,
	})
	if err != nil {
		t.Fatalf("Run basarisiz: %v\nstdout:\n%s", err, stdout.String())
	}

	result, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("cikis okunamadi: %v", err)
	}
	content := string(result)
	if !strings.Contains(content, "tomeepath=                 # trailing yorum") {
		t.Errorf("'atla' ile bilerek bos birakilan deger beklenen sekilde yazilmadi:\n%s", content)
	}
}

func TestRun_Interactive_IptalDurdurur(t *testing.T) {
	dir := t.TempDir()
	tmpl := writeTemp(t, dir, "env.sh", sampleTemplate)
	out := filepath.Join(dir, "out.sh")

	answers := "svc3ds\niptal\n"
	var stdout bytes.Buffer

	err := Run(Options{
		TemplatePath: tmpl,
		OutputPath:   out,
		In:           strings.NewReader(answers),
		Out:          &stdout,
	})
	if err == nil {
		t.Fatal("'iptal' sonrasi hata bekleniyordu")
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("'iptal' sonrasi cikis dosyasi yazilmamali")
	}
}

func TestReadAnswers_StripsQuotesAndComments(t *testing.T) {
	dir := t.TempDir()
	f := writeTemp(t, dir, "answers.env", `
# bu bir yorum
a=1
b="quoted value"
c='single quoted'

d=   trimmed
`)
	m, err := readAnswers(f)
	if err != nil {
		t.Fatalf("readAnswers basarisiz: %v", err)
	}
	want := map[string]string{"a": "1", "b": "quoted value", "c": "single quoted", "d": "trimmed"}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("answers[%q] = %q, istenen %q", k, m[k], v)
		}
	}
}
