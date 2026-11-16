// Package fcssync, steps/03_fcs/03_fcs_worker.sh dosyasinin Go
// karsiligidir. Bu paket, bu binary'yi calistiran HOST UZERINDE (host5 --
// FCS Central) calismasi beklenir: 3dsync-agent "fcs-run" olarak host0'dan
// host5'e SSH ile cagrilir, ve host5 UZERINDEN host7'ye (PROD/kaynak) rsync
// ile baglanir -- orijinal bash'teki IKI KATMANLI yapi (trigger: host0->host5,
// worker: host5->host7) BIREBIR korunmustur.
package fcssync

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

// Config, rsync cagrisi icin gereken tum degerleri tasir. Hicbir deger
// burada sabit degil -- hepsi caller'in env.sh'den okudugu degerlerle gelir.
type Config struct {
	SSHUser  string // rsync -e ssh icin kullanilan kullanici
	Host7    string // kaynak (PROD) host
	Src1     string // host7 uzerindeki kaynak yol
	Dest1    string // bu host'taki (host5) hedef yol
	MainPath string // SyncResult/ alt-dizininin olusturulacagi kok yol
}

// changedRe/deletedRe, rsync'in "-i" (itemize-changes) ciktisindaki
// eklenen/guncellenen ("<", ">", "c", "h" ile baslayan) ve silinen
// ("*deleting" ile baslayan) satirlari ayirt eder -- 03_fcs_worker.sh'deki
// grep -cE kaliplarinin birebir karsiligi.
var changedRe = regexp.MustCompile(`^[<>ch]`)
var deletedRe = regexp.MustCompile(`^\*deleting`)

// Sync, 03_fcs_worker.sh'nin TAM karsiligidir: SyncResult/ altina
// zaman damgali bir log dosyasi acar, "rsync -avz --delete -e ssh -i"
// ile ayna-modu kopyalama yapar (silinenler DAHIL -- bkz. orijinal
// scriptteki "DEV, PROD'un TAM AYNASI olur" notu), ve detayli dosya
// listesini EKRANA DEGIL log dosyasina yazar; ekrana sadece kisa bir
// eklenen/guncellenen + SILINEN sayisi ozeti basar.
func (c Config) Sync() error {
	syncResultDir := filepath.Join(c.MainPath, "SyncResult")
	if err := os.MkdirAll(syncResultDir, 0o775); err != nil {
		return fmt.Errorf("[fcssync] SyncResult dizini olusturulamadi: %w", err)
	}
	logPath := filepath.Join(syncResultDir, fmt.Sprintf("fcs_rsync_%s.log", time.Now().Format("20060102_150405")))

	src := fmt.Sprintf("%s@%s:%s", c.SSHUser, c.Host7, c.Src1)
	fmt.Printf("[fcssync] FCS kopyalaniyor (ayna modu, --delete aktif): %s -> %s\n", src, c.Dest1)
	fmt.Printf("[fcssync] (detayli dosya listesi arka planda logleniyor: %s)\n", logPath)

	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("[fcssync] log dosyasi olusturulamadi: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command("rsync", "-avz", "--delete", "-e", "ssh", "-i", src, c.Dest1)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	rsyncErr := cmd.Run()

	changed, deleted := countChanges(logPath)
	if rsyncErr == nil {
		fmt.Printf("[fcssync] FCS kopyalama tamamlandi -- eklenen/guncellenen: ~%d, SILINEN: %d. Detay: %s\n", changed, deleted, logPath)
		if deleted > 0 {
			fmt.Printf("[fcssync] NOT: %d dosya hedeften silindi (kaynakta artik yok). Silinen dosyalarin listesi icin: grep '^\\*deleting' %s\n", deleted, logPath)
		}
		return nil
	}
	return fmt.Errorf("[fcssync] rsync basarisiz oldu: %w (detay: %s)", rsyncErr, logPath)
}

// countChanges, log dosyasini satir satir okuyup CountChangesFromLines'a
// devreden ince bir I/O sarmalayicidir.
func countChanges(logPath string) (changed, deleted int) {
	f, err := os.Open(logPath)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return CountChangesFromLines(lines)
}

// CountChangesFromLines, rsync log satirlarindan degisen/silinen
// sayisini cikaran SAF (I/O'suz, kolayca test edilebilir) yardimcidir.
func CountChangesFromLines(lines []string) (changed, deleted int) {
	for _, line := range lines {
		if changedRe.MatchString(line) {
			changed++
		}
		if deletedRe.MatchString(line) {
			deleted++
		}
	}
	return changed, deleted
}
