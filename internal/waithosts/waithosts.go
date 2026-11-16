// Package waithosts, lib/wait_for_hosts.sh dosyasinin Go karsiligidir.
//
// AMAC (orijinalden korunan gerekce): reboot sonrasi "belki 300sn'de
// gelir" diye kor beklemek yerine, host'lara SSH ile GERCEKTEN geri
// baglanana kadar (makul bir ust sinira kadar) beklemek. Bu kontrol,
// script'i CALISTIRAN makineden (orchestrator) DOGRUDAN yapilir --
// host0 UZERINDEN degil, cunku reboot sirasinda hedef host'lar
// kapali/yeniden-acilma asamasinda olabilir.
package waithosts

import (
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Wait, wait_for_hosts() bash fonksiyonunun birebir karsiligidir:
// verilen host'lara (tekrarlar otomatik elenir) PARALEL olarak SSH
// baglanmayi dener, her biri icin maxWait suresine kadar bekler.
// Hepsi geri gelirse nil, en az biri gelmezse hata doner.
func Wait(user string, hosts []string, maxWait, interval time.Duration) error {
	seen := map[string]bool{}
	var unique []string
	for _, h := range hosts {
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		unique = append(unique, h)
	}
	if len(unique) == 0 {
		return nil
	}

	fmt.Printf("[wait_for_hosts] %d host icin SSH'in geri gelmesi bekleniyor (max %s)...\n", len(unique), maxWait)

	var wg sync.WaitGroup
	results := make([]error, len(unique))
	for i, h := range unique {
		wg.Add(1)
		go func(i int, h string) {
			defer wg.Done()
			results[i] = waitOne(user, h, maxWait, interval)
		}(i, h)
	}
	wg.Wait()

	var failed bool
	for _, err := range results {
		if err != nil {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("[wait_for_hosts] en az bir host, %s icinde SSH ile geri gelmedi", maxWait)
	}
	return nil
}

func waitOne(user, host string, maxWait, interval time.Duration) error {
	waited := time.Duration(0)
	target := user + "@" + host
	for waited < maxWait {
		cmd := exec.Command("ssh",
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=5",
			"-o", "StrictHostKeyChecking=no",
			target, "true")
		if err := cmd.Run(); err == nil {
			fmt.Printf("[wait_for_hosts] %s -> SSH geri geldi (%s)\n", host, waited)
			return nil
		}
		time.Sleep(interval)
		waited += interval
	}
	fmt.Printf("[wait_for_hosts] %s -> %s icinde SSH GERI GELMEDI\n", host, maxWait)
	return fmt.Errorf("%s SSH ile geri gelmedi", host)
}
