// Package orchestrate, main/run_full_sync.sh dosyasinin birebir Go
// karsiligidir. Buradaki asil deger KOD DEGIL, SIRALAMA/BAGIMLILIK
// BILGISIDIR: hangi adimin hangisinden once/sonra/paralel calismasi
// gerektigi, hangi adimlarin birbirine neden bagimli oldugu (ornegin
// FCS kopyalamanin DB restore ile PARALEL ama fullindex'ten ONCE bitmis
// olmasi gerektigi) -- bu bilgi, PLM ortamlariyla calisilarak ogrenilmis
// operasyonel tecrubedir ve asil korunmasi gereken budur.
//
// TUM adimlar artik migrate edildi: her adim, uzak sunucuda deploy
// edilmis 3dsync-agent binary'sinin kendi alt-komutunu (agentStep)
// cagirir -- musteri tarafinda calistirilmasi gereken duz metin bash
// script KALMADI (eski "legacy bash script yolu" tamamen kaldirildi;
// bkz. git gecmisinde "legacyStep" icin son referans).
package orchestrate

import (
	"fmt"
	"os"

	"khantech.com/3dsync-agent/internal/sshrun"
)

// Config, bash'teki env.sh + CLI bayraklarinin (--clear-acl,
// --with-3dindex) Go karsiligidir.
type Config struct {
	User        string
	Host0       string // ana kontrol/host0 -- digerlerine buradan ssh'leniyor
	MainPath    string
	ClearACL    bool
	With3DIndex bool
	AgentPath   string // uzak sunucuda deploy edilmis 3dsync-agent binary'sinin yolu
}

func FromEnvAndFlags(clearACL, with3DIndex bool) Config {
	mainpath := os.Getenv("mainpath")
	return Config{
		User:        os.Getenv("user"),
		Host0:       os.Getenv("host0"),
		MainPath:    mainpath,
		ClearACL:    clearACL,
		With3DIndex: with3DIndex,
		// DIKKAT (agentPath()/threedindex.Trigger() ile AYNI bug sinifi --
		// ikisi de gercek ortamda bulunup duzeltilmisti, burada da
		// KACMISTI): ham binary ("3dsync-agent") degil SARMALAYICI
		// ("3dsync-agent-run") kullanilmali -- orchestrate, host0'dan
		// KENDISINE (host0) SSH ile agentStep() cagirir; ham binary
		// cagrilirsa bu nested SSH oturumu env.sh'i hic kaynaklamaz,
		// 'user'/'exppath' gibi degiskenler BOS gelir.
		AgentPath: mainpath + "/3dsync-agent-run",
	}
}

// step, tek bir orkestrasyon adimini temsil eder: derlenmis agent
// alt-komutuna ssh ile cagri yapar.
type step struct {
	name   string
	target sshrun.Target
	cmd    string
}

func (c Config) target() sshrun.Target {
	return sshrun.Target{User: c.User, Host: c.Host0}
}

func (c Config) agentStep(name, subcommand string) step {
	return step{name: name, target: c.target(), cmd: fmt.Sprintf("%s %s", c.AgentPath, subcommand)}
}

func (s step) run() error {
	// RunLive kullanilir (Run degil) -- AYNI nedenle fanout1() de
	// degistirilmisti (bkz. sshrun.RunLive yorumu): orchestrate'in
	// adimlari (db-sync, fullindex gibi) dakikalarca surebilir, Run()
	// ile cikti sadece TAMAMEN bittiginde goruluyordu.
	_, err := sshrun.RunLive(s.target, s.cmd)
	if err != nil {
		return fmt.Errorf("adim '%s' basarisiz: %w", s.name, err)
	}
	return nil
}

// Sequence, run_full_sync.sh'in TAM sirasini ve bagimlilik mantigini
// uygular. Asamalar bash'teki yorumlarla birebir eslesecek sekilde
// numaralandi (bkz. README '0.x' referanslari orijinal script'te).
func (c Config) Sequence() error {
	if c.ClearACL {
		fmt.Println("==============================================================")
		fmt.Println(" --clear-acl VERILDI: fullindex adiminda ACL'ler TEMIZLENECEK.")
		fmt.Println("==============================================================")
	}

	// Adim 0: izin hazirlama -- ARTIK MIGRATE EDILDI ("prepare-permissions").
	// ESKI "deploy" adimi (setup/02_deploy.sh'in bash-agaci scp'leme kismi)
	// TAMAMEN KALDIRILDI: artik install.sh'in tek seferlik isi (binary +
	// wrapper + env.sh/secrets.sh dagitimi, bkz. o dosyadaki yorum) --
	// HER senkronda tekrar calistirilmasina gerek yok/yanlis olur.
	if err := c.agentStep("prepare-permissions", "prepare-permissions").run(); err != nil {
		return err
	}

	// Adim 1-2: servis/task durdurma -- ARTIK MIGRATE EDILDI (service-stop
	// ve task-stop trigger alt-komutlari, host2/3/4/5/9'a kendi SSH
	// fan-out'unu yapiyor -- bkz. cmd/3dsync-agent/main.go).
	if err := c.agentStep("service-stop", "service-stop").run(); err != nil {
		return err
	}
	if err := c.agentStep("task-stop", "task-stop").run(); err != nil {
		return err
	}

	// Adim 3 (FCS) ARKA PLANDA baslatilir -- store/config/servis
	// adimlariyla PARALEL calisir, SADECE fullindex'ten once bitmis
	// olmasi beklenir (asil bilgi tam burada: hangi adimin neyle
	// paralel, neyle sirali oldugu). ARTIK MIGRATE EDILDI ("fcs-trigger").
	fcsErr := make(chan error, 1)
	go func() {
		fcsErr <- c.agentStep("fcs-trigger", "fcs-trigger").run()
	}()
	fmt.Println("FCS kopyalama arka planda baslatildi -- store/config/servis adimlariyla PARALEL devam edilecek, sadece fullindex'ten once beklenecek.")

	// Adim 4+5: DB backup+restore -- ARTIK MIGRATE EDILDI ("db-sync"
	// trigger alt-komutu, host1'de sirayla backup-run + restore-run
	// calistirir). BU ADIM BEKLENIR (store/config adimlari matrix
	// DB'ye yaziyor, restore bitmeden calisamaz).
	if err := c.agentStep("db-sync", "db-sync").run(); err != nil {
		drainFCS(fcsErr)
		return fmt.Errorf("HATA: DB backup+restore basarisiz oldu, durduruluyor: %w", err)
	}
	fmt.Println("Step 4+5 Completed: Database Copy + Database Restore (FCS hala arka planda devam ediyor olabilir)")

	// Adim 6: store -- ARTIK MIGRATE EDILDI ("store-sync" trigger,
	// host3'te 06_1..06_8 MQL sirasini calistirir).
	if err := c.agentStep("store-sync", "store-sync").run(); err != nil {
		return err
	}

	// Adim 7 (location): TOGG'da "location" nesnesi olmadigi
	// DOGRULANDIGI icin bilerek ATLANIYOR -- kod silinmedi, sadece
	// cagrilmiyor. Baska bir musteride location nesneleri varsa, bu
	// adim burada geri eklenmeli.

	// Adim 8: config -- ARTIK TAMAMEN MIGRATE EDILDI ("config-sync"
	// trigger, host3'te beforemod->generate-config->modconfig->
	// aftermod->verify sirasini TEK SSH fan-out'u icinde calistirir).
	if err := c.agentStep("config-sync", "config-sync").run(); err != nil {
		return fmt.Errorf("HATA: config.xml uretim/dogrulama sirasi basarisiz: %w", err)
	}
	fmt.Println("Step 6+8 Completed: Store + Config degisikligi uygulandi ve URL dogrulandi")

	// Adim 10: log temizligi + servisleri baslat -- ARTIK MIGRATE EDILDI
	// ("clear-logs-start-services").
	if err := c.agentStep("clear-logs-start-services", "clear-logs-start-services").run(); err != nil {
		return err
	}

	// Adim 10.5: servis dogrulama -- ARTIK MIGRATE EDILDI
	// ("verify-services" trigger). Basarisizsa fullindex'e GECILMEZ.
	if err := c.agentStep("verify-services", "verify-services").run(); err != nil {
		drainFCS(fcsErr)
		return fmt.Errorf("HATA: Adim 10.5 (servis dogrulama) basarisiz oldu. Fullindex'e GECILMIYOR: %w", err)
	}
	fmt.Println("Step 10+10.5 Completed: Clear Logs + Start Services + Servis Dogrulama (hepsi ayakta)")

	// Fullindex'ten ONCE FCS'in gercekten bitmis olmasi sart.
	fmt.Println("Fullindex'ten once FCS kopyalamanin tamamlandigindan emin olunuyor...")
	if err := <-fcsErr; err != nil {
		return fmt.Errorf("HATA: FCS kopyalama basarisiz oldu. Fullindex'e GECILMIYOR: %w", err)
	}
	fmt.Println("FCS kopyalama tamamlandi (dogrulandi). Fullindex'e geciliyor.")

	// Adim 11: fullindex -- ARTIK MIGRATE EDILDI ("fullindex" trigger,
	// host3'te swap-monitor+validate+clearindex+stopindex+fullindex_start+
	// aftervalidate sirasini calistirir; --clear-acl bayragi iletilir).
	fullindexSub := "fullindex"
	if c.ClearACL {
		fullindexSub = "fullindex --clear-acl"
	}
	if err := c.agentStep("fullindex-trigger", fullindexSub).run(); err != nil {
		return fmt.Errorf("adim 'fullindex-trigger' basarisiz: %w", err)
	}

	// Adim 12: 3DIndex -- VARSAYILAN KAPALI, bayrakla acilir. ARTIK
	// MIGRATE EDILDI ("3dindex-trigger", host4'e SSH ile cagri yapar).
	if c.With3DIndex {
		fmt.Println("--with-3dindex VERILDI: Adim 12 (3DIndex) calistirilacak.")
		if err := c.agentStep("3dindex-trigger", "3dindex-trigger").run(); err != nil {
			return err
		}
	} else {
		fmt.Println("Adim 12 (3DIndex) ATLANDI (varsayilan -- --with-3dindex verilmedi).")
	}

	// Adim 13: scheduler restart -- ARTIK MIGRATE EDILDI.
	if err := c.agentStep("restart-scheduler", "restart-scheduler").run(); err != nil {
		return err
	}

	fmt.Println("Senkronizasyon tamamlandi.")
	return nil
}

func drainFCS(ch <-chan error) {
	select {
	case <-ch:
	default:
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
