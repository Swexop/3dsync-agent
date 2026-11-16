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

// Run, bash'teki:  ssh $user@$host "remoteCmd"
// ile birebir aynisidir -- stdout/stderr'i toplar, exit code'u dondurur.
func Run(t Target, remoteCmd string) (string, error) {
	cmd := exec.Command("ssh", t.String(), remoteCmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
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
	cmd := exec.Command("ssh", t.String(), remoteCmd)
	var out bytes.Buffer
	mw := io.MultiWriter(&out, os.Stdout)
	cmd.Stdout = mw
	cmd.Stderr = mw
	err := cmd.Run()
	return out.String(), err
}

// Copy, bash'teki:  scp localPath $user@$host:remotePath
// ile birebir aynisidir.
func Copy(localPath string, t Target, remotePath string) error {
	dest := fmt.Sprintf("%s:%s", t.String(), remotePath)
	cmd := exec.Command("scp", localPath, dest)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp %s -> %s basarisiz: %w (cikti: %s)", localPath, dest, err, out.String())
	}
	return nil
}
