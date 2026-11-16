package sshrun

import (
	"errors"
	"io"
	"testing"
)

// fakeRunner, commandRunner'in gercek ssh/scp binary'si CAGIRMAYAN bir
// test cift (test double)'idir -- hangi komutun hangi argumanlarla
// cagrildigini kaydeder, sabit bir cikti/hata dondurur.
type fakeRunner struct {
	gotName  string
	gotArgs  []string
	gotExtra io.Writer // Run()'da nil, RunLive()'da nil-DEGIL beklenir

	retOut string
	retErr error
}

func (f *fakeRunner) run(name string, args []string, extra io.Writer) (string, error) {
	f.gotName = name
	f.gotArgs = args
	f.gotExtra = extra
	return f.retOut, f.retErr
}

// withFakeRunner, testler boyunca paket-seviyesi 'runner' degiskenini
// gecici olarak degistirir ve test bitince GERI YUKLER -- paralel
// calismayan (bu paketin testleri paylasilan bir global degiskeni
// degistirdigi icin t.Parallel() KULLANILMAMALI) ama sirali calisan
// testler icin yeterlidir.
func withFakeRunner(t *testing.T, f *fakeRunner) {
	t.Helper()
	old := runner
	runner = f
	t.Cleanup(func() { runner = old })
}

func TestRun_BuildsSSHArgsAndReturnsOutput(t *testing.T) {
	f := &fakeRunner{retOut: "merhaba\n", retErr: nil}
	withFakeRunner(t, f)

	out, err := Run(Target{User: "svc3ds", Host: "10.0.0.5"}, "echo merhaba")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if out != "merhaba\n" {
		t.Errorf("out = %q, beklenen %q", out, "merhaba\n")
	}
	if f.gotName != "ssh" {
		t.Errorf("komut adi = %q, beklenen 'ssh'", f.gotName)
	}
	wantArgs := []string{"svc3ds@10.0.0.5", "echo merhaba"}
	if len(f.gotArgs) != 2 || f.gotArgs[0] != wantArgs[0] || f.gotArgs[1] != wantArgs[1] {
		t.Errorf("args = %v, beklenen %v", f.gotArgs, wantArgs)
	}
	if f.gotExtra != nil {
		t.Error("Run() canli yazim YAPMAMALI -- extra nil olmali")
	}
}

func TestRun_PropagatesError(t *testing.T) {
	wantErr := errors.New("exit status 1")
	f := &fakeRunner{retOut: "HATA: bir seyler ters gitti\n", retErr: wantErr}
	withFakeRunner(t, f)

	out, err := Run(Target{User: "u", Host: "h"}, "false")
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, beklenen %v", err, wantErr)
	}
	if out != "HATA: bir seyler ters gitti\n" {
		t.Errorf("basarisiz komutta bile toplanan cikti donmeli, out = %q", out)
	}
}

func TestRunLive_PassesNonNilExtraWriter(t *testing.T) {
	f := &fakeRunner{retOut: "ok", retErr: nil}
	withFakeRunner(t, f)

	if _, err := RunLive(Target{User: "u", Host: "h"}, "uzun-suren-komut"); err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if f.gotExtra == nil {
		t.Error("RunLive() canli yazim icin NON-NIL bir extra writer gecirmeli (Run()'dan FARKI tam burasi)")
	}
	if f.gotName != "ssh" {
		t.Errorf("komut adi = %q, beklenen 'ssh'", f.gotName)
	}
}

func TestCopy_BuildsSCPArgs(t *testing.T) {
	f := &fakeRunner{retOut: "", retErr: nil}
	withFakeRunner(t, f)

	err := Copy("/local/env.sh", Target{User: "svc3ds", Host: "host3"}, "/remote/config/env.sh")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if f.gotName != "scp" {
		t.Errorf("komut adi = %q, beklenen 'scp'", f.gotName)
	}
	wantArgs := []string{"/local/env.sh", "svc3ds@host3:/remote/config/env.sh"}
	if len(f.gotArgs) != 2 || f.gotArgs[0] != wantArgs[0] || f.gotArgs[1] != wantArgs[1] {
		t.Errorf("args = %v, beklenen %v", f.gotArgs, wantArgs)
	}
}

func TestCopy_WrapsErrorWithOutput(t *testing.T) {
	wantErr := errors.New("exit status 1")
	f := &fakeRunner{retOut: "scp: dosya bulunamadi", retErr: wantErr}
	withFakeRunner(t, f)

	err := Copy("/yok/dosya", Target{User: "u", Host: "h"}, "/remote/dosya")
	if err == nil {
		t.Fatal("hata beklenirdi, nil dondu")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("sarmalanmis hata orijinali icermeli (errors.Is basarisiz): %v", err)
	}
	if got := err.Error(); got == "" {
		t.Fatal("hata mesaji bos olmamali")
	}
}

func TestTarget_String(t *testing.T) {
	tg := Target{User: "svc3ds", Host: "10.0.0.5"}
	if got, want := tg.String(), "svc3ds@10.0.0.5"; got != want {
		t.Errorf("Target.String() = %q, beklenen %q", got, want)
	}
}
