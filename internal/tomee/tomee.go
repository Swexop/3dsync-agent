// Package tomee, lib/stop_tomee.sh dosyasinin Go karsiligidir. Bu
// paket, bu binary'yi calistiran HOST UZERINDE (uzak degil -- yerel
// process kontrolu) calismasi beklenir; yani 3dsync-agent'in kendisi
// TomEE'lerin calistigi sunucuya (host2/host3/host5/host9) deploy
// edilip orada "3dsync-agent service-stop <role>" olarak cagrilir.
package tomee

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// StopApp, stop_tomee_app() fonksiyonunun birebir karsiligidir: once
// nazikce bin/shutdown.sh, process gercekten (pgrep ile) kapanana kadar
// bekler, maxWait'i asarsa ZORLA (kill -9) kapatir.
func StopApp(label, tomeePath string, maxWait time.Duration) error {
	shutdownScript := tomeePath + "/bin/shutdown.sh"

	if !fileExecutable(shutdownScript) {
		fmt.Printf("[stop_tomee] %s: shutdown.sh bulunamadi (%s), atlaniyor.\n", label, shutdownScript)
		return nil
	}

	fmt.Printf("[stop_tomee] %s: shutdown.sh calistiriliyor...\n", label)
	_ = exec.Command(shutdownScript).Run()

	interval := 2 * time.Second
	waited := time.Duration(0)
	for waited < maxWait {
		if !processRunning(tomeePath) {
			fmt.Printf("[stop_tomee] %s: nazikce kapandi (%s icinde).\n", label, waited)
			return nil
		}
		time.Sleep(interval)
		waited += interval
	}

	pids := matchingPIDs(tomeePath)
	if len(pids) > 0 {
		fmt.Printf("[stop_tomee] %s: %s icinde kapanmadi, ZORLA kapatiliyor (PID: %s)...\n",
			label, maxWait, strings.Join(pids, " "))
		killArgs := append([]string{"-9"}, pids...)
		if err := exec.Command("sudo", append([]string{"kill"}, killArgs...)...).Run(); err != nil {
			_ = exec.Command("kill", killArgs...).Run()
		}
		time.Sleep(2 * time.Second)
		if processRunning(tomeePath) {
			return fmt.Errorf("[stop_tomee] %s: force-kill sonrasi HALA process gorunuyor", label)
		}
		fmt.Printf("[stop_tomee] %s: force-kill ile kapatildi.\n", label)
	}
	return nil
}

// StopNodeProcess, 01_stop_apptier.sh'deki 3DNotification (node process,
// TomEE degil) icin ozel mantigin karsiligidir: "node stop" + pgrep
// kontrolu + gerekirse force-kill.
func StopNodeProcess(label, nodeBinary, pgrepPattern string) error {
	fmt.Printf("[stop_tomee] %s: 'node stop' calistiriliyor...\n", label)
	_ = exec.Command(nodeBinary, "stop").Run()
	time.Sleep(3 * time.Second)

	if processRunning(pgrepPattern) {
		fmt.Printf("[stop_tomee] %s: hala ayakta, ZORLA kapatiliyor...\n", label)
		pids := matchingPIDs(pgrepPattern)
		if len(pids) > 0 {
			killArgs := append([]string{"-9"}, pids...)
			if err := exec.Command("sudo", append([]string{"kill"}, killArgs...)...).Run(); err != nil {
				_ = exec.Command("kill", killArgs...).Run()
			}
		}
	}
	return nil
}

func fileExecutable(path string) bool {
	err := exec.Command("test", "-x", path).Run()
	return err == nil
}

func processRunning(pattern string) bool {
	err := exec.Command("pgrep", "-f", pattern).Run()
	return err == nil
}

func matchingPIDs(pattern string) []string {
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		return nil
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	return fields
}
