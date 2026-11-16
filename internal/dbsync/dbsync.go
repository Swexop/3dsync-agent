// Package dbsync, steps/04_database/04_database_worker.sh (backup) ve
// steps/05_database_restore/05_restore_worker.sh (restore) dosyalarinin
// Go karsiligidir. Hicbir deger (host/path/DB adi/sifre) burada sabit
// degil -- hepsi Config struct'i uzerinden caller'in env.sh'den okudugu
// degerlerle gelir. Sifre asla log'a/ciktiya yazilmaz.
package dbsync

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Config, sqlcmd/ssh/scp cagrilari icin gereken tum degerleri tasir.
type Config struct {
	SSHUser   string // ssh/scp kullanicisi (orn. "x3ds")
	Host1     string // DB worker'in SSH ile cagrildigi/calistigi host (SQL_SERVER)
	Host8     string // backup dosyasinin alindigi/tasindigi host
	SQLCmdDir string // orn. /opt/mssql-tools/bin
	Username  string // orn. "sa"
	Password  string // ASLA log'lanmaz/yazdirilmaz
	DBName    string

	// Backup (04_database_worker.sh) icin:
	SQLPath2    string // host8 uzerinde backup'in yazildigi gecici dizin
	Src2        string // host8:$sqlpath2/..._auto.bak (scp kaynagi)
	Dest2       string // bu host'ta scp hedefi ($sqlpath)
	BackupFile2 string // host8 uzerindeki TSQL BACKUP hedef yolu

	// Restore (05_restore_worker.sh) icin:
	BackupFile string // bu hosttaki (host1/SQL_SERVER) .bak dosyasi yolu
	SQLData    string // mdf hedef dizini
	SQLLog     string // ldf hedef dizini
}

func (c Config) sqlcmdBin() string {
	if c.SQLCmdDir == "" {
		return "sqlcmd"
	}
	return c.SQLCmdDir + "/sqlcmd"
}

// Backup, 04_database_worker.sh'nin birebir karsiligidir:
//  1. host8'de hedef dizini herkese yazilabilir yapar (SQL Server'in
//     servis hesabinin, ssh kullanicisindan farkli olabilecegi icin --
//     bkz. orijinal scriptteki ayni gerekce),
//  2. BACKUP DATABASE komutunu sqlcmd ile calistirir,
//  3. backup dosyasinin izinlerini acar,
//  4. scp ile dosyayi bu host'a kopyalar.
func (c Config) Backup() error {
	fmt.Printf("[dbsync] host8 (%s) uzerinde hedef dizin hazirlaniyor: %s\n", c.Host8, c.SQLPath2)
	mkdirCmd := fmt.Sprintf("mkdir -p %s; sudo chmod 777 %s", c.SQLPath2, c.SQLPath2)
	if out, err := exec.Command("ssh", c.Host1Target(c.Host8), mkdirCmd).CombinedOutput(); err != nil {
		return fmt.Errorf("[dbsync] host8 dizin hazirligi basarisiz: %w (%s)", err, out)
	}

	tsqlBackup := fmt.Sprintf(`
USE master;
GO
BACKUP DATABASE %s
TO DISK='%s';
GO
`, c.DBName, c.BackupFile2)

	fmt.Println("[dbsync] BACKUP DATABASE calistiriliyor...")
	if err := c.runSQLInline(c.Host8, tsqlBackup); err != nil {
		return fmt.Errorf("[dbsync] BACKUP DATABASE basarisiz: %w", err)
	}

	fmt.Println("[dbsync] backup dosyasinin izinleri aciliyor...")
	chmodCmd := fmt.Sprintf("sudo chmod 777 %s", c.Src2)
	if out, err := exec.Command("ssh", c.Host1Target(c.Host8), chmodCmd).CombinedOutput(); err != nil {
		return fmt.Errorf("[dbsync] backup dosyasi izinleri acilamadi: %w (%s)", err, out)
	}

	fmt.Printf("[dbsync] scp ile kopyalaniyor: %s@%s:%s -> %s\n", c.SSHUser, c.Host8, c.Src2, c.Dest2)
	scpSrc := fmt.Sprintf("%s@%s:%s", c.SSHUser, c.Host8, c.Src2)
	if out, err := exec.Command("scp", scpSrc, c.Dest2).CombinedOutput(); err != nil {
		return fmt.Errorf("[dbsync] scp basarisiz: %w (%s)", err, out)
	}

	fmt.Println("[dbsync] Backup tamamlandi.")
	return nil
}

// Host1Target, "user@host" formatini olusturur (ssh/scp icin).
func (c Config) Host1Target(host string) string {
	return fmt.Sprintf("%s@%s", c.SSHUser, host)
}

// runSQLInline, bir T-SQL betigini sqlcmd ile calistirir. Orijinal
// bash "-i <(echo "$TSQL")" (process substitution) kullaniyordu; Go'da
// bunun karsiligi icin komutu bir gecici dosyaya yazip "-i dosya" ile
// veriyoruz (davranis AYNI, taninabilirlik/taşınabilirlik daha iyi).
func (c Config) runSQLInline(sqlServer, tsql string) error {
	tmp, err := os.CreateTemp("", "dbsync-*.sql")
	if err != nil {
		return fmt.Errorf("gecici sql dosyasi olusturulamadi: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(tsql); err != nil {
		tmp.Close()
		return fmt.Errorf("gecici sql dosyasina yazilamadi: %w", err)
	}
	tmp.Close()

	cmd := exec.Command(c.sqlcmdBin(), "-S", sqlServer, "-U", c.Username, "-P", c.Password,
		"-b", "-d", "master", "-i", tmp.Name())
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	return err
}

// queryFileList, "RESTORE FILELISTONLY" komutunu calistirip ham
// sqlcmd ciktisini dondurur.
func (c Config) queryFileList(sqlServer string) (string, error) {
	q := fmt.Sprintf("SET NOCOUNT ON; RESTORE FILELISTONLY FROM DISK='%s';", c.BackupFile)
	cmd := exec.Command(c.sqlcmdBin(), "-S", sqlServer, "-U", c.Username, "-P", c.Password,
		"-h", "-1", "-W", "-s", "|", "-Q", q)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("RESTORE FILELISTONLY basarisiz: %w", err)
	}
	return string(out), nil
}

// BuildMoveClauses, 05_restore_worker.sh'nin "RESTORE FILELISTONLY
// ciktisini satir satir okuyup dinamik MOVE ifadeleri uretme" mantiginin
// SAF (I/O'suz, kolayca test edilebilir) halidir. Dosya adlari asla
// hardcode edilmedi -- hepsi filelist ciktisindan geliyor.
//
// DUZELTME (orijinalden korunan kritik mantik): her logical dosya,
// turune (D=data, L=log) gore sqlData/sqlLog altinda yeniden
// adlandirilir; ilk log dosyasi "$DB_NAME.ldf" olur (eski DEV adlandirma
// kuraliyla uyumlu), fazladan log dosyalari (nadir) kendi mantiksal
// adiyla kalir -- isim CAKISMASINI onlemek icin.
func BuildMoveClauses(fileListOutput, dbName, sqlData, sqlLog string) (moveClauses string, fileCount int, err error) {
	scanner := bufio.NewScanner(strings.NewReader(fileListOutput))
	var clauses []string
	logFileIndex := 0

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, "|")
		if len(fields) < 3 {
			continue
		}
		logical := strings.TrimSpace(fields[0])
		fileType := strings.TrimSpace(fields[2])
		if logical == "" {
			continue
		}

		var newPath string
		switch fileType {
		case "D":
			newPath = fmt.Sprintf("%s/%s.mdf", sqlData, logical)
		case "L":
			logFileIndex++
			if logFileIndex == 1 {
				newPath = fmt.Sprintf("%s/%s.ldf", sqlLog, dbName)
			} else {
				newPath = fmt.Sprintf("%s/%s.ldf", sqlLog, logical)
			}
		default:
			fmt.Printf("  [uyari] taninmayan dosya turu ('%s') icin '%s' atlaniyor\n", fileType, logical)
			continue
		}

		fmt.Printf("  -> '%s' (%s) => %s\n", logical, fileType, newPath)
		clauses = append(clauses, fmt.Sprintf("MOVE '%s' TO '%s'", logical, newPath))
	}

	if len(clauses) == 0 {
		return "", 0, fmt.Errorf("backup dosyasindan hicbir gecerli logical file okunamadi -- RESTORE FILELISTONLY ciktisi:\n%s", fileListOutput)
	}
	return strings.Join(clauses, ", "), len(clauses), nil
}

// Restore, 05_restore_worker.sh'nin TUM sirasinin karsiligidir:
// FILELISTONLY oku -> MOVE ifadeleri olustur -> DB'yi offline al ->
// DB'yi sil -> dinamik MOVE'larla RESTORE et.
func (c Config) Restore() error {
	fmt.Println("[dbsync] Backup dosyasindaki gercek dosya listesi okunuyor (RESTORE FILELISTONLY)...")
	fileList, err := c.queryFileList(c.Host1)
	if err != nil {
		return fmt.Errorf("[dbsync] %w", err)
	}
	if strings.TrimSpace(fileList) == "" {
		return fmt.Errorf("[dbsync] RESTORE FILELISTONLY bos sonuc dondu")
	}

	moveClauses, fileCount, err := BuildMoveClauses(fileList, c.DBName, c.SQLData, c.SQLLog)
	if err != nil {
		return fmt.Errorf("[dbsync] %w", err)
	}
	fmt.Printf("[dbsync] %d dosya icin MOVE ifadesi olusturuldu.\n", fileCount)

	tsqlStop := fmt.Sprintf(`
USE master;
GO
ALTER DATABASE %s SET OFFLINE WITH ROLLBACK IMMEDIATE;
GO
`, c.DBName)
	tsqlDelete := fmt.Sprintf(`
USE master;
GO
DROP DATABASE %s;
GO
`, c.DBName)
	tsqlRestore := fmt.Sprintf(`
USE master;
GO
RESTORE DATABASE %s
FROM DISK='%s'
WITH REPLACE, %s;
GO
`, c.DBName, c.BackupFile, moveClauses)

	fmt.Println("[dbsync] Veritabani offline aliniyor...")
	if err := c.runSQLInline(c.Host1, tsqlStop); err != nil {
		// Orijinal script, stop/delete hatalarini kontrol etmeden devam
		// ediyordu (sadece son RESTORE_STATUS donduruluyordu) -- ayni
		// davranisi koruyoruz, sadece uyari basiyoruz.
		fmt.Printf("[dbsync] UYARI: offline alma adiminda hata (devam ediliyor): %v\n", err)
	}

	fmt.Println("[dbsync] Eski veritabani siliniyor...")
	if err := c.runSQLInline(c.Host1, tsqlDelete); err != nil {
		fmt.Printf("[dbsync] UYARI: silme adiminda hata (devam ediliyor): %v\n", err)
	}

	fmt.Println("[dbsync] RESTORE DATABASE (dinamik MOVE ile) calistiriliyor...")
	if err := c.runSQLInline(c.Host1, tsqlRestore); err != nil {
		return fmt.Errorf("[dbsync] RESTORE basarisiz: %w", err)
	}

	fmt.Println("[dbsync] Restore tamamlandi.")
	return nil
}
