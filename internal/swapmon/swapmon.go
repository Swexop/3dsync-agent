// Package swapmon, lib/ensure_swap.sh dosyasinin Go karsiligidir.
//
// Orijinal bash iki parcaydi: (1) ensure_swap_once -- esik altindaysa
// bir swapfile olusturup etkinlestirir; (2) arka plan izleme dongusu
// (setsid + nohup + PID dosyasi + "kill -- -$PID" ile process group'u
// birlikte oldurme). Go'da arka plan dongusu artik ayni process icinde
// bir goroutine -- disaridan PID dosyasiyla degil, context.Context ile
// iptal ediliyor; ayni "tamamen durdurulebilir" garanti, ekstra
// process-group karmasikligi olmadan saglaniyor.
package swapmon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// EnsureOnce, ensure_swap_once() fonksiyonunun birebir karsiligidir:
// bos (free) RAM GB cinsinden esigin altindaysa, baseDir altinda
// addGB boyutunda bir swapfile olusturur ve etkinlestirir. Esigin
// uzerindeyse veya swap zaten aktifse, hicbir sey yapmaz.
func EnsureOnce(thresholdGB, addGB int, baseDir string) error {
	freeGB, err := freeMemGB()
	if err != nil {
		return fmt.Errorf("bellek bilgisi okunamadi: %w", err)
	}
	if freeGB >= thresholdGB {
		fmt.Printf("[ensure_swap] Bos bellek %dGB (esik: %dGB) -- yeni swap gerekmiyor.\n", freeGB, thresholdGB)
		return nil
	}

	swapFile := filepath.Join(baseDir, fmt.Sprintf("3dsync_swap_%d", time.Now().Unix()))
	fmt.Printf("[ensure_swap] Bos bellek %dGB < esik %dGB -- %dGB swapfile olusturuluyor: %s\n",
		freeGB, thresholdGB, addGB, swapFile)

	if err := run("fallocate", "-l", fmt.Sprintf("%dG", addGB), swapFile); err != nil {
		// fallocate bazi dosya sistemlerinde calismaz -- dd'ye dus (orijinal
		// script'teki fallback mantigiyla ayni).
		fmt.Println("[ensure_swap] UYARI: fallocate basarisiz, dd ile deneniyor...")
		bs := "1M"
		count := strconv.Itoa(addGB * 1024)
		if err := run("dd", "if=/dev/zero", "of="+swapFile, "bs="+bs, "count="+count); err != nil {
			return fmt.Errorf("swapfile olusturulamadi (fallocate ve dd basarisiz): %w", err)
		}
	}

	if err := os.Chmod(swapFile, 0600); err != nil {
		return fmt.Errorf("swapfile izinleri ayarlanamadi: %w", err)
	}
	if err := run("mkswap", swapFile); err != nil {
		return fmt.Errorf("mkswap basarisiz: %w", err)
	}
	if err := run("swapon", swapFile); err != nil {
		return fmt.Errorf("swapon basarisiz: %w", err)
	}

	fmt.Printf("[ensure_swap] Swapfile aktif edildi: %s\n", swapFile)
	return nil
}

// Cleanup, orijinal script'in temizlik adimidir: baseDir altindaki
// "3dsync_swap_*" ile baslayan tum swapfile'lari swapoff edip siler.
func Cleanup(baseDir string) error {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		// DUZELTME (gercek ortamda bulundu): orijinal bash'teki
		// "for swapfile in ${base_dir}/swap_autosync_*.img" glob'u,
		// baseDir hic YOKSA bile HATA VERMEZ -- glob eslesmezse
		// dongu hic calismaz, "Temizlenecek ek swap dosyasi
		// bulunamadi" denir. Eskiden burada baseDir yoklugu bir
		// HATA olarak donduruluyordu (swap_base_dir hic set
		// edilmemis/varsayilan "/db" olup da diskte olmayan bir
		// ortamda, fullindex'in SONUNDA gereksiz bir UYARI
		// basiyordu). Orijinal davranisla ayni: dizin yoksa,
		// temizlenecek bir sey yok demektir, hata degil.
		if os.IsNotExist(err) {
			fmt.Printf("[ensure_swap] %s yok -- temizlenecek ek swap dosyasi bulunamadi (hic eklenmemisti, sistem zaten oncekiyle ayni).\n", baseDir)
			return nil
		}
		return fmt.Errorf("baseDir okunamadi: %w", err)
	}
	var lastErr error
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "3dsync_swap_") {
			continue
		}
		path := filepath.Join(baseDir, e.Name())
		fmt.Printf("[ensure_swap] Temizleniyor: %s\n", path)
		if err := run("swapoff", path); err != nil {
			fmt.Printf("[ensure_swap] UYARI: swapoff basarisiz (%s): %v\n", path, err)
			lastErr = err
		}
		if err := os.Remove(path); err != nil {
			fmt.Printf("[ensure_swap] UYARI: dosya silinemedi (%s): %v\n", path, err)
			lastErr = err
		}
	}
	return lastErr
}

// RunMonitor, arka plan izleme dongusunun karsiligidir -- orijinaldeki
// "setsid nohup ... & echo $! > pidfile" yerine, caller context'i iptal
// edene kadar periyodik olarak EnsureOnce'u cagiran bir goroutine olarak
// calisir. ctx iptal edildiginde dongu TAMAMEN durur (orijinaldeki
// "kill -- -$PID" process-group-kapatma garantisinin in-process esdegeri).
func RunMonitor(ctx context.Context, thresholdGB, addGB int, baseDir string, checkInterval time.Duration) {
	fmt.Printf("[ensure_swap] Izleme dongusu baslatildi (her %s bir kontrol).\n", checkInterval)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("[ensure_swap] Izleme dongusu durduruldu.")
			return
		case <-ticker.C:
			if err := EnsureOnce(thresholdGB, addGB, baseDir); err != nil {
				fmt.Printf("[ensure_swap] UYARI: periyodik kontrol basarisiz: %v\n", err)
			}
		}
	}
}

func freeMemGB() (int, error) {
	out, err := exec.Command("free", "-g").Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.HasPrefix(fields[0], "Mem") {
			// free -g sutunlari: total used free shared buff/cache available
			avail := fields[len(fields)-1]
			n, err := strconv.Atoi(avail)
			if err != nil {
				return 0, fmt.Errorf("beklenmeyen 'free' ciktisi: %q", line)
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("'free -g' ciktisinda Mem satiri bulunamadi")
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	return err
}
