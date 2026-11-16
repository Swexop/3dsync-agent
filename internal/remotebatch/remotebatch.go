// Package remotebatch, lib/remote_batch.sh dosyasinin Go karsiligidir.
//
// Orijinal bash mantigi: ayni host'a ait komutlar ';' ile birlestirilip
// TEK ssh oturumunda sirayla calistirilir (bir komut hata verse bile
// ayni host'taki sonraki komutlar yine de calisir -- '&&' DEGIL ';').
// Farkli host'lar birbirinden bagimsiz oldugu icin PARALEL calistirilir.
//
// Go tarafinda ayni semantigi, TEK ssh orumu + ';' string birlestirme
// yerine, HER KOMUT ICIN AYRI bir ssh cagrisi ile ama "devam et" (hata
// olsa bile sonraki komuta gec) mantigiyla koruyoruz -- bu, hata
// ayiklamayi kolaylastirir (hangi komutun basarisiz oldugu net gorulur)
// ve orijinal ";" davranisiyla sonuc olarak AYNIDIR (bir onceki komutun
// basarisi sonrakini etkilemez).
package remotebatch

import (
	"fmt"
	"sync"

	"khantech.com/3dsync-agent/internal/sshrun"
)

type queued struct {
	host string
	cmd  string
}

// Batch, remote_queue/remote_flush ciftinin Go karsiligidir. Sifir
// degerli bir Batch kullanima hazirdir (bash'teki global degiskenlerin
// aksine, ayni anda birden fazla Batch kullanilabilir -- bash'teki
// "kuyrugu sifirla, ayni oturumda tekrar kullanilabilsin" notuyla
// ayni amaca, ama global state olmadan ulasir).
type Batch struct {
	user  string
	items []queued
}

func New(user string) *Batch {
	return &Batch{user: user}
}

// Queue, bash'teki remote_queue <host> <komut> karsiligidir. Bos host
// sessizce atlanir (bash'teki "UYARI: host tanimli degil" davranisiyla
// ayni -- script'i patlatmaz).
func (b *Batch) Queue(host, cmd string) {
	if host == "" {
		fmt.Printf("[remote_batch] UYARI: host tanimli degil, komut atlaniyor: %s\n", cmd)
		return
	}
	if cmd == "" {
		return
	}
	b.items = append(b.items, queued{host: host, cmd: cmd})
}

// Flush, bash'teki remote_flush karsiligidir: ayni host'a ait komutlar
// SIRAYLA (ama bagimsiz ssh cagrilariyla), farkli host'lar PARALEL
// calistirilir. Kuyruk, cagridan sonra sifirlanir.
func (b *Batch) Flush() error {
	// Host sirasini koru, komutlari host'a gore grupla.
	order := []string{}
	grouped := map[string][]string{}
	seen := map[string]bool{}
	for _, it := range b.items {
		if !seen[it.host] {
			seen[it.host] = true
			order = append(order, it.host)
		}
		grouped[it.host] = append(grouped[it.host], it.cmd)
	}
	b.items = nil

	if len(order) == 0 {
		return nil
	}

	if len(order) == 1 {
		// Tek sunucu senaryosu: paralellestirmeye gerek yok.
		return b.runHostSequential(order[0], grouped[order[0]])
	}

	// Cok sunucu senaryosu: her host bagimsiz, paralel calistir.
	var wg sync.WaitGroup
	errs := make([]error, len(order))
	for i, h := range order {
		wg.Add(1)
		go func(i int, h string) {
			defer wg.Done()
			fmt.Printf("[remote_batch] %s@%s -> arka planda baslatildi\n", b.user, h)
			errs[i] = b.runHostSequential(h, grouped[h])
		}(i, h)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			fmt.Printf("[remote_batch] UYARI: %s uzerindeki komutlardan biri hata dondu: %v\n", order[i], err)
			return err
		}
	}
	return nil
}

func (b *Batch) runHostSequential(host string, cmds []string) error {
	t := sshrun.Target{User: b.user, Host: host}
	var lastErr error
	for _, cmd := range cmds {
		out, err := sshrun.Run(t, cmd)
		if out != "" {
			fmt.Print(out)
		}
		if err != nil {
			// Bash'teki ';' semantigi: hata olsa bile SONRAKI komuta devam
			// edilir -- ama son hatayi caller'a bildirmek icin saklariz.
			fmt.Printf("[remote_batch] UYARI: %s@%s uzerinde '%s' basarisiz: %v\n", b.user, host, cmd, err)
			lastErr = err
		}
	}
	return lastErr
}
