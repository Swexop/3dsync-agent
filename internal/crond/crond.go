// Package crond, steps/02_task_stop.sh ve steps/13_restart_scheduler.sh
// dosyalarinin Go karsiligidir -- host3/host4 uzerinde crond servisini
// durdurma/yeniden baslatma. Bu binary'nin hedef host UZERINDE
// calistirilmasi beklenir (orijinaldeki remote_batch ile tetiklenen
// uzak script'lerle AYNI konum).
package crond

import (
	"fmt"
	"os/exec"
)

func runSudoService(action string) error {
	cmd := exec.Command("sudo", "service", "crond", action)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	if err != nil {
		return fmt.Errorf("[crond] 'sudo service crond %s' basarisiz: %w", action, err)
	}
	return nil
}

// Stop, 02_task_stop.sh'nin bu host'taki karsiligidir.
func Stop() error {
	fmt.Println("[crond] crond durduruluyor...")
	return runSudoService("stop")
}

// Restart, 13_restart_scheduler.sh'nin bu host'taki karsiligidir.
func Restart() error {
	fmt.Println("[crond] crond yeniden baslatiliyor...")
	return runSudoService("restart")
}
