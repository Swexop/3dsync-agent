// Package configgen, steps/08_config/08_1_5_generate_config.sh dosyasinin
// birebir Go karsiligidir. Bu adim, kaynak ortamdan alinan
// config_Before.xml uzerinde urlconf1 -> urlconf2 (ve doluysa
// urlconf3 -> urlconf4) metin degisimini uygulayarak hedef ortam icin
// gecerli config.xml dosyasini URETIR.
//
// Bu dosya, gercek musteri URL/host degerlerini icermez -- sadece
// DONUSUM KURALINI (hangi deger hangisiyle degistirilir, hangi sirada,
// hangi durumlarda uyari verilir) icerir. Degerlerin kendisi, mevcut
// config/env.sh / config/profiles/*.sh mekanizmasindan oldugu gibi
// okunmaya devam eder -- bu kati bir "config'i de goster" degil, sadece
// "donusum mantigini sakla" hedefiyle tasarlandi.
package configgen

import (
	"fmt"
	"os"
	"strings"
)

// Config, bu adimin env.sh'den okudugu degiskenlerin karsiligidir.
type Config struct {
	MainPath string // $mainpath
	URLConf1 string // $urlconf1 (kaynak, ör. "3dspacefts")
	URLConf2 string // $urlconf2 (hedef, ör. "3dspaceftsqa")
	URLConf3 string // $urlconf3 (opsiyonel ikinci degisim - kaynak)
	URLConf4 string // $urlconf4 (opsiyonel ikinci degisim - hedef)
}

// FromEnv, bash'teki ". config/env.sh" ile ayni degiskenleri okur.
// Gercek degerler hala env.sh / secrets.sh icinde kaliyor -- burada
// sadece okuma yapiyoruz, degerleri koda GOMMUYORUZ.
func FromEnv() Config {
	return Config{
		MainPath: os.Getenv("mainpath"),
		URLConf1: os.Getenv("urlconf1"),
		URLConf2: os.Getenv("urlconf2"),
		URLConf3: os.Getenv("urlconf3"),
		URLConf4: os.Getenv("urlconf4"),
	}
}

// Generate, 08_1_5_generate_config.sh'in tam karsiligidir:
//  1. $mainpath/SyncResult/config_Before.xml dosyasini okur
//  2. urlconf1->urlconf2 ve (doluysa) urlconf3->urlconf4 metin
//     degisimini uygular (bash'teki sed_escape + sed -i mantiginin
//     Go karsiligi -- strings.ReplaceAll, sed gibi regex degil DUZ
//     METIN degisimi yaptigi icin ozel karakter kacislamaya gerek yok)
//  3. Sonucu $mainpath/SyncResult/config.xml olarak yazar
//
// Donus degeri: kac degisim uygulandigini (0, 1 veya 2) ve hata (varsa).
func (c Config) Generate() (applied int, err error) {
	beforePath := c.MainPath + "/SyncResult/config_Before.xml"
	targetPath := c.MainPath + "/SyncResult/config.xml"

	data, err := os.ReadFile(beforePath)
	if err != nil {
		return 0, fmt.Errorf("HATA: %s okunamadi (once 08_1_beforemod.sh calismis olmali): %w", beforePath, err)
	}
	if len(data) == 0 {
		return 0, fmt.Errorf("HATA: %s bos", beforePath)
	}

	content := string(data)

	if c.URLConf1 != "" && c.URLConf2 != "" {
		content = strings.ReplaceAll(content, c.URLConf1, c.URLConf2)
		applied++
		fmt.Printf("config.xml: '%s' -> '%s' degisimi uygulandi.\n", c.URLConf1, c.URLConf2)
	}

	if c.URLConf3 != "" && c.URLConf4 != "" {
		content = strings.ReplaceAll(content, c.URLConf3, c.URLConf4)
		applied++
		fmt.Printf("config.xml: '%s' -> '%s' degisimi uygulandi.\n", c.URLConf3, c.URLConf4)
	}

	if applied == 0 {
		fmt.Fprintln(os.Stderr, "UYARI: urlconf1/urlconf2 (ve urlconf3/urlconf4) bos -- config.xml, "+
			"config_Before.xml ile BIREBIR ayni yazildi, hicbir metin degisimi yapilmadi.")
	}

	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return applied, fmt.Errorf("HATA: %s yazilamadi: %w", targetPath, err)
	}

	fmt.Printf("config.xml uretildi: %s\n", targetPath)
	return applied, nil
}
