// Package threedindex, steps/12_3dindex/*.sh dosyalarinin Go
// karsiligidir.
//
// ONEMLI: 12_3dindex_run.sh'nin SON satiri olan "timeout 3600" bilerek
// ve BIREBIR tasindi -- bu, orijinal script'te "timeout 3600 <komut>"
// olmasi gerekirken KOMUTSUZ birakilmis bir NO-OP/HATA (timeout,
// calistiracagi komut olmadan hata verir ve hicbir sey beklemez).
// Projenin kendi CHANGELOG notuna gore bu, MUSTERI ONAYI beklenen,
// bilinen ama BILEREK DUZELTILMEMIS bir bug'dir -- bu port sirasinda
// "sessizce iyilestirilmedi". Duzeltme musteri onayiyla gelirse,
// RunIndex() icindeki ilgili satir guncellenmelidir.
package threedindex

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"khantech.com/3dsync-agent/internal/sshrun"
)

type Config struct {
	SSHUser string
	Host4   string
	ExpPath string
	// MainPath, bu binary'nin host4'te deploy edildigi dizin -- Start()
	// fonksiyonu host4 UZERINDE calisirken kullanilir (Trigger() host3'ten
	// ssh ile host4'teki "3dsync-agent threedindex-run" cagrisini tetikler).
	MainPath string
}

// Trigger, 12_3dindex_trigger.sh'nin karsiligidir -- host4'teki eski
// 3DIndexes dizinini "_old" olarak yedekler, sonra host4'te
// "threedindex-run" alt-komutunu (bu binary'nin kendisini, Start()'a
// karsilik gelen) tetikler.
func (c Config) Trigger() error {
	t := sshrun.Target{User: c.SSHUser, Host: c.Host4}
	volBase := filepath.Join(c.ExpPath, "VolumeSearch")

	fmt.Println("[3dindex] host4: eski 3DIndexes_old siliniyor")
	if out, err := sshrun.Run(t, "rm -rf "+filepath.Join(volBase, "3DIndexes_old")); err != nil {
		return fmt.Errorf("[3dindex] 3DIndexes_old silinemedi: %w (%s)", err, out)
	}

	fmt.Println("[3dindex] host4: 3DIndexes -> 3DIndexes_old tasiniyor")
	mvCmd := fmt.Sprintf("mv %s %s", filepath.Join(volBase, "3DIndexes"), filepath.Join(volBase, "3DIndexes_old"))
	if out, err := sshrun.Run(t, mvCmd); err != nil {
		return fmt.Errorf("[3dindex] 3DIndexes tasinamadi: %w (%s)", err, out)
	}

	fmt.Println("[3dindex] host4: 3dsync-agent-run threedindex-run tetikleniyor")
	// DIKKAT (agentPath() ile AYNI bug sinifi -- main.go'da bir kez
	// bulunup duzeltilmisti, burada KACMIS): ham binary ("3dsync-agent")
	// degil SARMALAYICI ("3dsync-agent-run") cagrilmali -- yoksa bu
	// nested SSH oturumu env.sh'i hic kaynaklamaz, "user"/"exppath" gibi
	// degiskenler BOS gelir. Gercek bir ortamda tam bu sekilde bulundu.
	remoteCmd := filepath.Join(c.MainPath, "3dsync-agent-run") + " threedindex-run"
	out, err := sshrun.Run(t, remoteCmd)
	if out != "" {
		fmt.Print(out)
	}
	return err
}

// RunIndex, 12_3dindex_run.sh'nin karsiligidir -- bu binary host4
// UZERINDE calisirken cagrilir (12_3dindex_start.sh'nin eski "source et"
// davranisinin karsiligi: ayri bir process degil, dogrudan ayni binary
// icinde bir fonksiyon).
func (c Config) RunIndex() error {
	workDir := filepath.Join(c.ExpPath, "VolumeSearch", "linux_a64", "code", "command")
	envDir := filepath.Join(c.ExpPath, "VolumeSearch", "CATEnv") + "/"

	fmt.Printf("[3dindex] calisma dizini: %s\n", workDir)
	cmd := exec.Command("./catstart",
		"-run", "BBDAdminPlayer -EnvName 3DIndex_DefaultEnv",
		"-env", "Env.txt",
		"-direnv", envDir,
	)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	if err != nil {
		return fmt.Errorf("[3dindex] catstart basarisiz: %w", err)
	}

	// BILEREK KORUNAN BUG: orijinaldeki son satir "timeout 3600" idi --
	// calistiracagi bir komut OLMADAN, yani bu komut HER ZAMAN hemen
	// hata verip doner (gercek bir 3600sn bekleme YAPMAZ). Musteri onayi
	// gelmeden bu "duzeltilmiyor" -- hata BILEREK goz ardi ediliyor,
	// orijinal script de bunu kontrol etmiyordu.
	_ = exec.Command("timeout", "3600").Run()

	fmt.Println("[3dindex] Tamamlandi (NOT: yukaridaki 'timeout 3600' adimi bilinen, musteri onayi bekleyen bir no-op'tur).")
	return nil
}
