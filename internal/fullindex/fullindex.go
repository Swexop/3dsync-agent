// Package fullindex, steps/11_fullindex/*.sh dosyalarinin Go
// karsiligidir -- 3DSync'teki EN KRITIK, geri donusu en zor adim.
// Bu binary host3'te calisir (orijinal remote_queue "$host3" ile
// aynı); host4'e olan "cvinit.sh stop/start" cagrilari SSH ile yapilir.
package fullindex

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"khantech.com/3dsync-agent/internal/mqlstore"
	"khantech.com/3dsync-agent/internal/sshrun"
	"khantech.com/3dsync-agent/internal/swapmon"
)

// Config, fullindex sirasinin ihtiyac duydugu tum degerleri tasir.
type Config struct {
	MQL      mqlstore.Config // ExpPath/MainPath/credentials buradan gelir
	SSHUser  string
	Host4    string // 3DSpaceIndex sunucusu (cvinit.sh)
	ClearACL bool   // CLEAR_ACL=1 ile ayni -- varsayilan false (GUVENLI)
}

func (c Config) runMQLSimple(mqlCmd, outName string) error {
	return c.MQL.RunNamed(mqlCmd, outName)
}

// Validate, 11_1_validate.sh / 11_6_aftervalidate.sh'nin ortak
// mantigidir -- hangi asamada cagrildigina gore farkli dosya adlariyla.
func (c Config) validate(outName string) error {
	return c.runMQLSimple(`validate searchindex;`, outName)
}

// ClearACL, 11_2_clearacl.sh karsiligidir. DIKKAT: "clear accesslist
// all;" TUM ACL'leri siler -- geri donusu zor, istenmeyen bir islemdir.
// Orijinaldeki gibi SADECE c.ClearACL=true (CLEAR_ACL=1) ISE cagrilir --
// varsayilan olarak atlanir (bkz. Run() icindeki kosul).
func (c Config) clearACL() error {
	fmt.Println("UYARI: CLEAR_ACL=1 -- accesslist TEMIZLENECEK (clear accesslist all;)")
	return c.runMQLSimple(`clear accesslist all;`, "acl_clear")
}

func (c Config) clearIndex() error {
	return c.runMQLSimple(`clear searchindex;`, "clearindex")
}

func (c Config) stopIndex() error {
	return c.runMQLSimple(`stop searchindex;`, "stop_index")
}

// fullIndexStart, 11_5_fullindex_start.sh'nin birebir karsiligidir --
// host4'teki cvinit.sh'yi stop/start eder (aralarinda TAM OLARAK
// orijinaldeki 50s/50s/100s beklemelerle), sonra MQL ile
// "start searchindex mode full;" tetikler. Bu zamanlamalar bilerek
// BIREBIR korunuyor -- degistirmek riskli bir operasyonel karardir ve
// kullanicinin onayi olmadan "iyilestirilmez".
func (c Config) fullIndexStart() error {
	t := sshrun.Target{User: c.SSHUser, Host: c.Host4}
	cvinit := filepath.Join(c.MQL.ExpPath, "3DSpaceIndex", "linux_a64", "cv", "data", "bin", "cvinit.sh")
	runDir := filepath.Join(c.MQL.ExpPath, "3DSpaceIndex", "linux_a64", "cv", "data", "run")

	fmt.Println("[fullindex] host4: cvinit.sh stop")
	if out, err := sshrun.Run(t, cvinit+" stop"); err != nil {
		return fmt.Errorf("[fullindex] cvinit.sh stop basarisiz: %w (%s)", err, out)
	}
	time.Sleep(50 * time.Second)

	fmt.Println("[fullindex] host4: run dizini temizleniyor")
	if out, err := sshrun.Run(t, "rm -rf "+runDir+"/*"); err != nil {
		return fmt.Errorf("[fullindex] run dizini temizlenemedi: %w (%s)", err, out)
	}
	time.Sleep(50 * time.Second)

	fmt.Println("[fullindex] host4: cvinit.sh start")
	if out, err := sshrun.Run(t, cvinit+" start"); err != nil {
		return fmt.Errorf("[fullindex] cvinit.sh start basarisiz: %w (%s)", err, out)
	}
	time.Sleep(100 * time.Second)

	fmt.Println("[fullindex] MQL: start searchindex mode full")
	return c.runMQLSimple(`start searchindex mode full;`, "full_index")
}

// Run, 11_fullindex_trigger.sh'nin TUM sirasinin karsiligidir:
// swap-monitor baslat -> validate -> (opsiyonel) clearacl -> clearindex
// -> stopindex -> fullindex_start -> 300sn bekle -> aftervalidate ->
// swap-monitor durdur + swap dosyalarini temizle.
//
// Swap izleyicisi artik ayri bir process degil, bu fonksiyon icinde
// context ile yonetilen bir goroutine -- surecin HICBIR ANINDA
// izlemesiz kalmamasi garantisi AYNI sekilde korunuyor, ama PID
// dosyasi/"kill -- -$PID" karmasikligi olmadan.
func (c Config) Run(swapBaseDir string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go swapmon.RunMonitor(ctx, 10, 30, swapBaseDir, 60*time.Second)

	steps := []struct {
		name string
		fn   func() error
	}{
		{"validate (once)", func() error { return c.validate("validate_before") }},
	}
	if c.ClearACL {
		steps = append(steps, struct {
			name string
			fn   func() error
		}{"clearacl", c.clearACL})
	}
	steps = append(steps,
		struct {
			name string
			fn   func() error
		}{"clearindex", c.clearIndex},
		struct {
			name string
			fn   func() error
		}{"stopindex", c.stopIndex},
		struct {
			name string
			fn   func() error
		}{"fullindex_start", c.fullIndexStart},
	)

	for _, s := range steps {
		fmt.Printf("[fullindex] %s calistiriliyor...\n", s.name)
		if err := s.fn(); err != nil {
			cancel()
			return fmt.Errorf("[fullindex] %s adiminda hata: %w", s.name, err)
		}
	}

	fmt.Println("[fullindex] fullindex_start sonrasi 300sn bekleniyor...")
	time.Sleep(300 * time.Second)

	fmt.Println("[fullindex] aftervalidate calistiriliyor...")
	if err := c.validate("validate_after"); err != nil {
		cancel()
		return fmt.Errorf("[fullindex] aftervalidate basarisiz: %w", err)
	}

	// Swap izleyicisini durdur ve eklenmis swap dosyalarini temizle
	// (11_9_swap_monitor_stop.sh ile AYNI -- fullindex bitince temizlenir).
	cancel()
	fmt.Println("[fullindex] Fullindex bitti -- eklenmis swap dosyalari temizleniyor...")
	if err := swapmon.Cleanup(swapBaseDir); err != nil {
		fmt.Printf("[fullindex] UYARI: swap temizligi tam basarili olmadi: %v\n", err)
	}

	fmt.Println("[fullindex] Tamamlandi.")
	return nil
}
