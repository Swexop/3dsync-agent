// Package sshrun, bash tarafindaki "ssh $user@$host \"komut\"" ve
// "scp kaynak $user@$host:hedef" cagrilarinin dogrudan karsiligidir.
// Bilerek kendi SSH istemcimizi yazmiyoruz: mevcut host-trust (anahtar
// tabanli SSH + sudo NOPASSWD) kurulumunu degistirmeden, sistemdeki
// `ssh`/`scp` komutlarini cagiriyoruz. Boylece setup/00_bootstrap_trust.sh
// ile kurulan guven hic bozulmuyor; sadece hangi komutun, hangi sirada,
// hangi bagimlilikla calistigi artik duz metin bash yerine derlenmis
// binary icinde.
package sshrun

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Target, bash'teki "$user@$host" ikilisinin karsiligi.
type Target struct {
	User string
	Host string
}

func (t Target) String() string {
	return fmt.Sprintf("%s@%s", t.User, t.Host)
}

// commandRunner, GERCEK dis komut calistirmayi (exec.Command+Run) SOYUTLAR
// -- Run/RunLive/Copy'nin "argumanlari olustur -> calistir -> ciktiyi
// topla/hata sarmala" bilesim mantigi, gercek bir ssh/scp binary'sine
// (ve gercek bir ag baglantisina) ihtiyac duymadan birim testiyle
// dogrulanabilsin diye. Varsayilan uygulama (execRunner) gercek
// os/exec kullanir; testler bu paket-seviyesi degiskeni (runner) sahte
// bir uygulamayla degistirir.
type commandRunner interface {
	// run, name komutunu args ile calistirir. extra nil DEGILSE (RunLive
	// davranisi), komutun CALISIRKEN birlesik stdout+stderr'i ANINDA
	// extra'ya da yazilir (Run()'da extra nil'dir -- hicbir sey canli
	// yazilmaz, hepsi sessizce toplanir). Her iki durumda da toplanan
	// TAM metin ve exit hatasi dondurulur.
	run(name string, args []string, extra io.Writer) (string, error)
}

type execRunner struct{}

func (execRunner) run(name string, args []string, extra io.Writer) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	if extra != nil {
		mw := io.MultiWriter(&out, extra)
		cmd.Stdout = mw
		cmd.Stderr = mw
	} else {
		cmd.Stdout = &out
		cmd.Stderr = &out
	}
	err := cmd.Run()
	return out.String(), err
}

var runner commandRunner = execRunner{}

// Run, bash'teki:  ssh $user@$host "remoteCmd"
// ile birebir aynisidir -- stdout/stderr'i toplar, exit code'u dondurur.
func Run(t Target, remoteCmd string) (string, error) {
	return runner.run("ssh", []string{t.String(), remoteCmd}, nil)
}

// RunLive, Run ile AYNI SONUCU dondurur (toplam stdout+stderr metni +
// exit hatasi) ama FARKLI OLARAK, komut CALISIRKEN cikisini ANINDA
// (satir satir, bash'teki "ssh ... komut" gibi canli) bu surecin kendi
// stdout/stderr'ine de yazar. Run() ise hicbir sey yazdirmaz, hepsini
// sessizce toplar ve komut TAMAMEN BITTIKTEN SONRA caller'in elle
// print etmesini bekler.
//
// NEDEN EKLENDI: gercek bir ortamda, uzun suren bir "trigger" komutu
// (orn. db-sync -> db-restore-run, 3.8GB'lik bir RESTORE DATABASE)
// Run() ile cagrildiginda, kullanici dakikalarca HICBIR CIKTI GORMEDI
// -- bu, surecin "donmus/kilitlenmis" oldugu izlenimi yaratti ve
// kullaniciyi, ortasinda (offline/drop/restore adimlarindan birinde)
// Ctrl-C yapmaya itecek riskli bir durum olusturdu. fanout1(), artik
// uzun surebilecek/yikici (db-sync, store-sync, config-sync,
// service-stop, verify-services, fullindex) tetikleyiciler icin bunu
// kullaniyor.
func RunLive(t Target, remoteCmd string) (string, error) {
	return runner.run("ssh", []string{t.String(), remoteCmd}, os.Stdout)
}

// Copy, bash'teki:  scp localPath $user@$host:remotePath
// ile birebir aynisidir.
func Copy(localPath string, t Target, remotePath string) error {
	dest := fmt.Sprintf("%s:%s", t.String(), remotePath)
	out, err := runner.run("scp", []string{localPath, dest}, nil)
	if err != nil {
		return fmt.Errorf("scp %s -> %s basarisiz: %w (cikti: %s)", localPath, dest, err, out)
	}
	return nil
}
