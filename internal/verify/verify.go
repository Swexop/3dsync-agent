// Package verify, steps/10_5_verify_services/*.sh dosyalarinin Go
// karsiligidir. Her fonksiyon, bu binary'yi calistiran host UZERINDE
// (yerel process/servis kontrolu) calisir -- yani ilgili role'e deploy
// edilmis 3dsync-agent "verify <role>" olarak cagrilir.
package verify

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// systemChecker, bu paketin yaptigi UC tur gercek isletim-sistemi
// kontrolunu (dosya/yol varligi, process arama, keyfi bir komutun
// basari/basarisizligi) SOYUTLAR -- asagidaki aggregation/wiring
// mantigi, gercek dosya sistemine veya gercek calisan process'lere
// dokunmadan birim testiyle dogrulanabilsin diye. Varsayilan uygulama
// (execChecker) gercek os/exec kullanir; testler paket-seviyesi 'sys'
// degiskenini sahte bir uygulamayla degistirir.
type systemChecker interface {
	pathExists(path string) bool
	pathExecutable(path string) bool
	processRunning(pattern string) bool
	commandSucceeds(name string, args ...string) bool
}

type execChecker struct{}

func (execChecker) pathExists(path string) bool {
	return exec.Command("test", "-e", path).Run() == nil
}

func (execChecker) pathExecutable(path string) bool {
	return exec.Command("test", "-x", path).Run() == nil
}

func (execChecker) processRunning(pattern string) bool {
	return exec.Command("pgrep", "-f", pattern).Run() == nil
}

func (execChecker) commandSucceeds(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

var sys systemChecker = execChecker{}

// retryUntil, bu paketteki UC yerde (WaitForProcess, Index, LB)
// tekrarlanan "check() true donene kadar interval araliklarla tekrar
// dene, en fazla maxWait kadar" deseninin ORTAK halidir -- MERKEZI
// olarak (kucuk sure degerleriyle, hizlica) test edilir; boylece her
// cagiran yer kendi 120s/180s/60s sabitiyle GERCEK ZAMAN beklemeden bu
// mantigin dogrulugundan emin olunabilir.
func retryUntil(maxWait, interval time.Duration, check func() bool) (ok bool, waited time.Duration) {
	for waited < maxWait {
		if check() {
			return true, waited
		}
		time.Sleep(interval)
		waited += interval
	}
	return false, waited
}

// WaitForProcess, verify_apptier.sh / verify_space.sh icindeki
// wait_for_process() fonksiyonunun birebir karsiligidir -- TEK FARKLA:
// pattern bir YOL (/ ile basliyorsa), tam maxWait boyunca pgrep ile
// bosu bosuna beklemek yerine, once o yolun DISK'TE VAR OLUP
// OLMADIGINI hemen kontrol eder. Yol yoksa, "servis henuz baslamadi"
// ile "env.sh'deki yol yanlis/bu host'ta bu rol yok" birbirinden
// ayirt edilebilsin diye ANINDA, acik bir hata doner -- gercek bir
// test ortaminda tam 2 dakika bosa beklendigi fark edilince eklendi.
func WaitForProcess(label, pattern string, maxWait, interval time.Duration) error {
	if strings.HasPrefix(pattern, "/") && !sys.pathExists(pattern) {
		return fmt.Errorf("  HATA %s -- YOL BULUNAMADI: '%s' bu host'ta yok (env.sh'deki yol hatali olabilir, ya da bu rol bu host'ta kurulu degil)", label, pattern)
	}

	ok, waited := retryUntil(maxWait, interval, func() bool {
		return sys.processRunning(pattern)
	})
	if ok {
		fmt.Printf("  OK   %s (%s icinde ayakta)\n", label, waited)
		return nil
	}
	return fmt.Errorf("  HATA %s -- yol var ama %s icinde process CALISMIYOR (pattern: %s)", label, maxWait, pattern)
}

// AppTier, verify_apptier.sh (host2) karsiligidir.
func AppTier(commentPath, dashboardPath, passportPath, fedsearchPath, swymPath string) error {
	const maxWait = 120 * time.Second
	const interval = 5 * time.Second
	fmt.Println("--- host2 (AppTier) servis dogrulama ---")

	var failed error
	checks := []struct{ label, pattern string }{
		{"Comment", commentPath},
		{"Dashboard", dashboardPath},
		{"Passport", passportPath},
		{"FedSearch", fedsearchPath},
		{"Swym", swymPath},
		{"Notification", "3DNotification"},
	}
	for _, c := range checks {
		if err := WaitForProcess(c.label, c.pattern, maxWait, interval); err != nil {
			fmt.Println(err)
			failed = err
		}
	}
	return failed
}

// Space, verify_space.sh (host3) karsiligidir.
func Space(spacePath, internalPath string) error {
	const maxWait = 120 * time.Second
	const interval = 5 * time.Second
	fmt.Println("--- host3 (Space) servis dogrulama ---")

	var failed error
	checks := []struct{ label, pattern string }{
		{"3DSpace", spacePath},
		{"3DSpaceInternal", internalPath},
	}
	for _, c := range checks {
		if err := WaitForProcess(c.label, c.pattern, maxWait, interval); err != nil {
			fmt.Println(err)
			failed = err
		}
	}
	return failed
}

// FCS, verify_fcs.sh (host5) karsiligidir.
func FCS(fcsCentralPath string) error {
	fmt.Println("--- host5 (FCS Central) servis dogrulama ---")
	if err := WaitForProcess("FCS Central", fcsCentralPath, 120*time.Second, 5*time.Second); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// FCSRemote, verify_fcs_remote.sh (host9, opsiyonel) karsiligidir.
func FCSRemote(fcsRemotePath string) error {
	fmt.Println("--- host9 (FCS Remote) servis dogrulama ---")
	if err := WaitForProcess("FCS Remote", fcsRemotePath, 120*time.Second, 5*time.Second); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// Index, verify_index.sh (host4) karsiligidir. DOGRULANDI (kullanici):
// "cvinit.sh status" bu ortamda destekleniyor -- basarisizsa GERCEK
// HATA sayilir (eskiden sadece uyari veriyordu).
func Index(cvinitPath string) error {
	const maxWait = 180 * time.Second
	const interval = 5 * time.Second
	fmt.Println("--- host4 (3DSpaceIndex) servis dogrulama ---")

	if !sys.pathExecutable(cvinitPath) {
		err := fmt.Errorf("  HATA cvinit.sh bulunamadi/calistirilamaz: %s", cvinitPath)
		fmt.Println(err)
		return err
	}

	ok, waited := retryUntil(maxWait, interval, func() bool {
		return sys.commandSucceeds(cvinitPath, "status")
	})
	if ok {
		fmt.Printf("  OK   3DSpaceIndex (cvinit.sh status, %s icinde)\n", waited)
		return nil
	}
	err := fmt.Errorf("  HATA 3DSpaceIndex -- %s icinde 'cvinit.sh status' basarili olmadi", maxWait)
	fmt.Println(err)
	return err
}

// defaultLBCandidates, host6'da yuk dengeleyici olarak kullanilabilecek
// BILINEN servis adlaridir (HAProxy yaygin ama Apache/nginx de olabilir
// -- gercek bir ortamda HAProxy'nin hic kurulu olmadigi, Apache'nin
// kullanildigi bir durumla karsilasildi). Hicbiri env.sh degeri degil,
// sadece denenen servis isimleri.
var defaultLBCandidates = []string{"haproxy", "httpd", "apache2", "nginx"}

// LB, verify_lb.sh (host6) karsiligidir. override bos ise, bilinen
// LB servislerinin HEPSİ denenir (otomatik tespit); override
// verilirse (orn. env.sh'deki "lbservice" degeri ile main.go'dan
// gecirilir) SADECE o servis kontrol edilir.
func LB(override string) error {
	const maxWait = 60 * time.Second
	const interval = 3 * time.Second
	fmt.Println("--- host6 (Load Balancer) servis dogrulama ---")

	candidates := defaultLBCandidates
	if override != "" {
		candidates = []string{override}
		fmt.Printf("  (env.sh'deki 'lbservice' degeri kullaniliyor: %s)\n", override)
	} else {
		fmt.Printf("  (lbservice env.sh'de belirtilmedi, su adaylar deneniyor: %s)\n", strings.Join(candidates, ", "))
	}

	var matched string
	ok, waited := retryUntil(maxWait, interval, func() bool {
		for _, svc := range candidates {
			if serviceActive(svc) {
				matched = svc
				return true
			}
		}
		return false
	})
	if ok {
		fmt.Printf("  OK   %s aktif (%s icinde)\n", matched, waited)
		return nil
	}

	var err error
	if override != "" {
		err = fmt.Errorf("  HATA '%s' -- %s icinde aktif olmadi", override, maxWait)
	} else {
		err = fmt.Errorf("  HATA hicbir bilinen LB servisi (%s) %s icinde aktif bulunamadi -- ortaminizda farkli bir isim kullaniliyorsa, env.sh'ye 'lbservice=<servis-adi>' ekleyip tekrar deneyin", strings.Join(candidates, ", "), maxWait)
	}
	fmt.Println(err)
	return err
}

func serviceActive(name string) bool {
	return sys.commandSucceeds("systemctl", "is-active", "--quiet", name) ||
		sys.commandSucceeds("service", name, "status")
}
