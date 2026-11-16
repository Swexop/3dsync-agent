// Package mqlstore, steps/06_store/*.sh (STORE/plmx/Image Store MQL
// degisiklikleri) ve steps/08_config/08_1..4 (searchindex config MQL
// degisiklikleri + dogrulama) dosyalarinin Go karsiligidir.
//
// Hepsi MQL (Matrix Query Language) komutlarini "mql -c" ile cagirir;
// bu paket o cagrilari ve dosya I/O'sunu Go'dan yapar, ama MQL'in
// KENDISINI (3DEXPERIENCE kurulumunun bir parcasi) tekrar yazmaz --
// sadece hangi komutun hangi sirada ve hangi parametreyle calistirildigi
// mantigini tasir. Hicbir deger (host/path/URL) burada sabit degil;
// hepsi caller'in Config struct'indan (env var'lardan) gelir.
package mqlstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Config, mql cagrilari icin gereken ortam degerlerini tasir --
// hepsi caller tarafindan env.sh'den (os.Getenv) okunup buraya verilir.
type Config struct {
	ExpPath   string // orn. 3DEXPERIENCE kurulum koku
	MainPath  string // AutoSync/3DSync calisma dizini
	PlmxPath  string
	PlmxURL   string
	StorePath string
	StoreURL  string
	ImagePath string
	ImageURL  string
}

func (c Config) mqlBin() string {
	return filepath.Join(c.ExpPath, "3DSpace", "scripts", "mql")
}

func (c Config) credFile() string {
	return filepath.Join(c.ExpPath, "3DSpace", "credentials", "creator")
}

func (c Config) syncResult() string {
	return filepath.Join(c.MainPath, "SyncResult")
}

// runMQL, bir MQL komut dizisini calistirir ve stdout/stderr'i, orijinal
// script'lerdeki ile ayni adlandirma seklinde ".out"/".err" dosyalarina
// yazar (operasyon ekiplerinin alisik oldugu dosya adlari korunur).
func (c Config) runMQL(mqlCmd, outName string) error {
	fullCmd := fmt.Sprintf(`set context user creator filename "%s"; verbose on; %s`, c.credFile(), mqlCmd)
	cmd := exec.Command(c.mqlBin(), "-c", fullCmd)
	out, err := cmd.Output()
	var errOut []byte
	if ee, ok := err.(*exec.ExitError); ok {
		errOut = ee.Stderr
	}

	sr := c.syncResult()
	if werr := os.MkdirAll(sr, 0755); werr != nil {
		return fmt.Errorf("SyncResult dizini olusturulamadi: %w", werr)
	}
	_ = os.WriteFile(filepath.Join(sr, outName+".out"), out, 0644)
	_ = os.WriteFile(filepath.Join(sr, outName+".err"), errOut, 0644)

	if err != nil {
		return fmt.Errorf("mql komutu basarisiz (%s): %w", outName, err)
	}
	return nil
}

// RunNamed, disaridan (orn. fullindex paketinden) genel amacli MQL
// komutlari calistirmak icin kullanilan kucuk bir disa-acik sarmalayicidir
// (ortak dosya adlandirma/SyncResult yazma mantigini tekrar yazmamak icin).
func (c Config) RunNamed(mqlCmd, outName string) error {
	return c.runMQL(mqlCmd, outName)
}

// --- 06_store sirasi (06_1 .. 06_8) ---

// BeforeMod, 06_1_beforemod.sh karsiligidir: degisiklik oncesi tum
// store'larin durumunu listeler (denetim/rollback icin referans).
func (c Config) BeforeMod() error {
	return c.runMQL(`list store * select path fcsurl host;`, "listBeforeModification_n1")
}

// PLMXPathMod, 06_2_plmxpathmod.sh karsiligidir.
func (c Config) PLMXPathMod() error {
	cmd := fmt.Sprintf(`modify store "plmx" path "%s";`, c.PlmxPath)
	return c.runMQL(cmd, "modifyStorePLMXPath_n1")
}

// PLMXURLMod, 06_3_plmxurlmod.sh karsiligidir.
func (c Config) PLMXURLMod() error {
	cmd := fmt.Sprintf(`modify store "plmx" fcs "%s";`, c.PlmxURL)
	return c.runMQL(cmd, "modifyStorePLMXFCSUrl_n1")
}

// StorePathMod, 06_4_storepathmod.sh karsiligidir.
func (c Config) StorePathMod() error {
	cmd := fmt.Sprintf(`modify store "STORE" path "%s";`, c.StorePath)
	return c.runMQL(cmd, "modifyStoreStorePath_n1")
}

// StoreURLMod, 06_5_storeurlmod.sh karsiligidir.
func (c Config) StoreURLMod() error {
	cmd := fmt.Sprintf(`modify store "STORE" fcs "%s";`, c.StoreURL)
	return c.runMQL(cmd, "modifyStoreStoreFCSUrl_n1")
}

// ImagePathMod, 06_6_imagepathmod.sh karsiligidir.
func (c Config) ImagePathMod() error {
	cmd := fmt.Sprintf(`modify store 'Image Store' path "%s";`, c.ImagePath)
	return c.runMQL(cmd, "modifyStoreImageStorePath_n1")
}

// ImageURLMod, 06_7_imageurlmod.sh karsiligidir.
func (c Config) ImageURLMod() error {
	cmd := fmt.Sprintf(`modify store 'Image Store' fcs "%s";`, c.ImageURL)
	return c.runMQL(cmd, "modifyStoreImageStoreFCSUrl_n1")
}

// AfterMod, 06_8_aftermod.sh karsiligidir: degisiklik sonrasi durumu
// tekrar listeler (before/after karsilastirmasi icin).
func (c Config) AfterMod() error {
	return c.runMQL(`list store * select path fcsurl host;`, "listAfterModification_n1")
}

// StoreSequence, 06_store_trigger.sh'nin TUM sirasinin karsiligidir --
// orijinaldeki remote_queue sirasini AYNEN korur. Herhangi bir adim
// basarisiz olursa hemen durur (orijinal script de ';' ile zincirlese
// de, burada ilk hatada durmak daha guvenli -- bir path/url modify
// basarisiz olduysa sonrakilere devam etmenin degeri yok).
func (c Config) StoreSequence() error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"beforemod", c.BeforeMod},
		{"plmx-path", c.PLMXPathMod},
		{"plmx-url", c.PLMXURLMod},
		{"store-path", c.StorePathMod},
		{"store-url", c.StoreURLMod},
		{"image-path", c.ImagePathMod},
		{"image-url", c.ImageURLMod},
		{"aftermod", c.AfterMod},
	}
	for _, s := range steps {
		fmt.Printf("[mqlstore] %s calistiriliyor...\n", s.name)
		if err := s.fn(); err != nil {
			return fmt.Errorf("[mqlstore] %s adiminda hata: %w", s.name, err)
		}
	}
	fmt.Println("[mqlstore] 06_store sirasi tamamlandi.")
	return nil
}

// --- 08_config MQL kismi (08_1, 08_2, 08_3 -- 08_1_5 configgen
// paketinde, 08_4 ConfigConfig icin asagidaki VerifyConfig fonksiyonunda) ---

// BeforeModConfig, 08_1_beforemod.sh karsiligidir: degisiklik oncesi
// searchindex ayarini config_Before.xml'e yazdirir (configgen paketi
// bu dosyayi girdisi olarak kullanir).
func (c Config) BeforeModConfig() error {
	beforeFile := filepath.Join(c.syncResult(), "config_Before.xml")
	cmd := fmt.Sprintf(`print system searchindex file %s;`, beforeFile)
	return c.runMQL(cmd, "printSystemSearchindex_Before_n1")
}

// ModConfig, 08_2_modconfig.sh karsiligidir: configgen paketinin
// urettigi SyncResult/config.xml'i ENOFramework AppInstall/Programs
// hedefine kopyalar ve searchindex'e "force" ile set eder.
// targetConfigDir, orijinaldeki
// "$exppath/3DSpace/Apps/BusinessProcessServices/.../AppInstall/Programs"
// yoludur -- bu, 3DEXPERIENCE surum numarasina (V6R2021x gibi) bagli
// oldugundan sabit kod YAZILMADAN caller'dan parametre olarak alinir.
func (c Config) ModConfig(targetConfigDir string) error {
	src := filepath.Join(c.syncResult(), "config.xml")
	dst := filepath.Join(targetConfigDir, "config.xml")

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("config.xml okunamadi (once configgen calistirilmali): %w", err)
	}
	if err := os.MkdirAll(targetConfigDir, 0755); err != nil {
		return fmt.Errorf("hedef dizin olusturulamadi: %w", err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("config.xml kopyalanamadi: %w", err)
	}

	cmd := fmt.Sprintf(`set system searchindex file '%s' force;`, dst)
	return c.runMQL(cmd, "setSystemSearchindex_n1")
}

// AfterModConfig, 08_3_aftermod.sh karsiligidir: degisiklik sonrasi
// durumu config_After.xml'e yazdirir.
func (c Config) AfterModConfig() error {
	afterFile := filepath.Join(c.syncResult(), "config_After.xml")
	cmd := fmt.Sprintf(`print system searchindex file %s;`, afterFile)
	return c.runMQL(cmd, "printSystemSearchindex_After_n1")
}

// VerifyConfig, 08_4_verify_config.sh'nin KRITIK dogrulama mantiginin
// birebir karsiligidir.
//
// DIKKAT (orijinalden korunan bug-fix notu): urlConf1 genelde urlConf2'nin
// bir ON EKI'dir (orn. "3dspacefts" -> "3dspaceftsqa"), yani basit bir
// "icerir mi" kontrolu HER ZAMAN true doner. Dogru yontem: urlConf2'nin
// TUM gecmislerini metinden cikarip, geriye hala urlConf1 kaliyor mu
// bakmaktir -- kalirsa, GERCEKTEN degistirilmemis (naked) bir yer
// vardir. Orijinal bash'teki "buyuk dosyayi once bir shell degiskenine
// alip sonra grep'e vermek" (argument list too long + SESSIZCE yutulan
// hata) bug'u burada YOKTUR -- Go'da dosya hep bellekte/[]byte olarak
// isleniyor, shell komut-satiri argumani limiti hic devreye girmiyor.
func VerifyConfig(afterFilePath, urlConf1, urlConf2 string) error {
	fmt.Println("--- config.xml URL dogrulama ---")

	info, err := os.Stat(afterFilePath)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("  HATA: %s bulunamadi veya bos -- aftermod once calismis olmali", afterFilePath)
	}

	if urlConf2 == "" {
		fmt.Printf("  UYARI: urlConf2 bos, karsilastirma yapilamiyor. Elle kontrol edin: %s\n", afterFilePath)
		return nil
	}

	data, err := os.ReadFile(afterFilePath)
	if err != nil {
		return fmt.Errorf("  HATA: %s okunamadi: %w", afterFilePath, err)
	}
	content := string(data)

	if !strings.Contains(content, urlConf2) {
		return fmt.Errorf("  HATA: config.xml icinde beklenen hedef URL ('%s') BULUNAMADI -- degisiklik uygulanmamis olabilir: %s",
			urlConf2, afterFilePath)
	}

	// urlConf2'nin tum gecmislerini cikar, geriye urlConf1 kaliyor mu bak.
	remainder := strings.ReplaceAll(content, urlConf2, "")
	if strings.Contains(remainder, urlConf1) {
		return fmt.Errorf("  HATA: config.xml icinde HALA degistirilmemis (naked) '%s' geciyor -- bazi yerler degismis olabilir ama hepsi degil: %s",
			urlConf1, afterFilePath)
	}

	fmt.Printf("  OK   config.xml icinde hedef URL ('%s') dogrulandi, degistirilmemis '%s' kalintisi yok.\n", urlConf2, urlConf1)
	return nil
}
