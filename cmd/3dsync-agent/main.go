// 3dsync-agent -- KhanTech 3DSync urununun derlenmis orkestrasyon/worker
// binary'si. Amac: musteri tarafina duz metin bash script olarak
// dagitilan "asil deger tasiyan" mantigi (adim siralamasi/bagimliligi,
// MQL/SQL komut sekansi, config donusum kurali, servis durdurma/dogrulama
// mantigi) tek bir derlenmis binary icine almak.
//
// Ayni binary HER HOST'a (host0/host1/host2/host3/host4/host5/host6/
// host8/host9) deploy edilir ve iki tur alt-komutu vardir:
//
//   - "trigger" komutlari (orn. store-sync, verify-services, fullindex):
//     host0'da calisir, orijinal bash trigger script'leri gibi hedef
//     host'lara SSH ile fan-out yapar.
//   - "-run" komutlari (orn. store-sync-run, verify-run, fullindex-run):
//     o hedef host UZERINDE calistirilmasi beklenir -- yerel process/
//     mql/sqlcmd cagrilari yapar, baska hosta SSH ATMAZ (fullindex-run
//     ve 3dindex-trigger'in host4'e yaptigi tek-atlamali istisnalar
//     haric -- bunlar da orijinal script'lerin kendi ic mantigidir).
//
// Her alt-komut, ayni ortam degiskenlerini (mainpath, user, host0..9,
// exppath, urlconf1..4, vb.) config/env.sh + config/secrets.sh'den
// miras alir -- bu dosyalar DEGISMEDI, sadece onlari OKUYAN mantik
// artik derlenmis. Hicbir host/path/URL/sifre degeri bu dosyada SABIT
// DEGIL -- hepsi os.Getenv ile okunuyor.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"khantech.com/3dsync-agent/internal/configgen"
	"khantech.com/3dsync-agent/internal/crond"
	"khantech.com/3dsync-agent/internal/dbsync"
	"khantech.com/3dsync-agent/internal/fcssync"
	"khantech.com/3dsync-agent/internal/fullindex"
	"khantech.com/3dsync-agent/internal/initconfig"
	"khantech.com/3dsync-agent/internal/mqlstore"
	"khantech.com/3dsync-agent/internal/orchestrate"
	"khantech.com/3dsync-agent/internal/postsync"
	"khantech.com/3dsync-agent/internal/preflight"
	"khantech.com/3dsync-agent/internal/sshrun"
	"khantech.com/3dsync-agent/internal/threedindex"
	"khantech.com/3dsync-agent/internal/tomee"
	"khantech.com/3dsync-agent/internal/verify"
	"khantech.com/3dsync-agent/internal/waithosts"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "orchestrate":
		cmdOrchestrate(args)
	case "generate-config":
		cmdGenerateConfig()
	case "version":
		fmt.Println(versionString())

	case "validate-config":
		cmdValidateConfig(args)
	case "init-config":
		cmdInitConfig(args)

	// --- store (06_store) ---
	case "store-sync":
		dieOn(fanout1("store-sync", mustEnv("host3"), "store-sync-run"))
	case "store-sync-run":
		dieOn(mqlCfg().StoreSequence())

	// --- config (08_config, 08_1_5 generate-config haric) ---
	case "config-sync":
		cmdConfigSync()
	case "config-sync-run":
		dieOn(cmdConfigSyncRun())
	case "config-beforemod-run":
		dieOn(mqlCfg().BeforeModConfig())
	case "config-modconfig-run":
		dieOn(mqlCfg().ModConfig(configTargetDir()))
	case "config-aftermod-run":
		dieOn(mqlCfg().AfterModConfig())
	case "config-verify-run":
		afterFile := mustEnv("mainpath") + "/SyncResult/config_After.xml"
		dieOn(mqlstore.VerifyConfig(afterFile, mustEnv("urlconf1"), os.Getenv("urlconf2")))

	// --- servis durdurma (01_service_stop) ---
	case "service-stop":
		cmdServiceStop()
	case "service-stop-run":
		cmdServiceStopRun(args)

	// --- FCS kopyalama (03_fcs) ---
	case "fcs-trigger":
		cmdFCSTrigger()
	case "fcs-run":
		cmdFCSRun()

	// --- crond (02_task_stop / 13_restart_scheduler) ---
	case "task-stop":
		dieOn(fanoutAll("task-stop", []string{mustEnv("host3"), mustEnv("host4")}, "crond-stop-run"))
	case "restart-scheduler":
		dieOn(fanoutAll("restart-scheduler", []string{mustEnv("host3"), mustEnv("host4")}, "crond-restart-run"))
	case "crond-stop-run":
		dieOn(crond.Stop())
	case "crond-restart-run":
		dieOn(crond.Restart())

	// --- database (04_database / 05_database_restore) ---
	case "db-sync":
		cmdDBSync()
	case "db-backup-run":
		dieOn(dbCfg().Backup())
	case "db-restore-run":
		dieOn(dbCfg().Restore())

	// --- servis dogrulama (10_5_verify_services) ---
	case "verify-services":
		cmdVerifyServices()
	case "verify-run":
		cmdVerifyRun(args)

	// --- fullindex (11_fullindex) ---
	case "fullindex":
		cmdFullindexTrigger(args)
	case "fullindex-run":
		cmdFullindexRun(args)

	// --- 3dindex (12_3dindex) ---
	case "3dindex-trigger":
		dieOn(threedindexCfg().Trigger())
	case "threedindex-run":
		dieOn(threedindexCfg().RunIndex())

	// --- reboot sonrasi host bekleme (lib/wait_for_hosts.sh) ---
	case "wait-for-hosts":
		cmdWaitForHosts(args)

	// --- log temizligi + servis baslatma (10_clear_logs_and_start_services) ---
	case "clear-logs-start-services":
		cmdClearLogsStartServices()

	// --- izin hazirlama (eski setup/01_prepare_permissions.sh +
	// setup/02_deploy.sh'nin HALA GECERLI olan kismi -- bkz. cmdPreparePermissions
	// yorumu: dosya dagitimi/dos2unix artik install.sh'in isi, burada
	// SADECE DB ve config dizinlerindeki calisma-zamani izinleri kalir) ---
	case "prepare-permissions":
		cmdPreparePermissions()

	default:
		usage()
		os.Exit(1)
	}
}

// ============================================================
// Ortak yardimcilar
// ============================================================

func mustEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fmt.Fprintf(os.Stderr, "HATA: '%s' ortam degiskeni bos -- once config/env.sh (ve gerekirse secrets.sh) kaynaklanmali.\n", name)
		os.Exit(1)
	}
	return v
}

func dieOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sshUser() string { return mustEnv("user") }

// buildVersion, derleme aninda -ldflags "-X main.buildVersion=..." ile
// disaridan verilebilir (orn. git tag/commit). Verilmezse "dev" kalir --
// bu, "paketlenmemis/elle derlenmis" bir binary'yi "dev" etiketiyle
// ayirt etmeyi saglar (productization: surum izlenebilirligi).
var buildVersion = "dev"

func versionString() string {
	return fmt.Sprintf("3dsync-agent %s", buildVersion)
}

// cmdValidateConfig, "validate-config" alt-komutunun gerceklestirmesidir
// -- bkz. internal/preflight paketi. Hicbir senkron adimini CALISTIRMAZ,
// sadece env.sh'in (bu host'ta source edilmis haliyle) eksik/sablon
// deger icerip icermedigini VE host'lara SSH ile erisilebilirligini
// kontrol eder. Yeni bir musteri kurulumunda install.sh'den ONCE
// calistirilmasi onerilir.
func cmdValidateConfig(args []string) {
	fs := flag.NewFlagSet("validate-config", flag.ExitOnError)
	skipSSH := fs.Bool("skip-ssh", false, "SSH erisim testlerini atla (sadece env.sh icerigini kontrol et)")
	timeout := fs.Int("ssh-timeout", 5, "her host icin SSH baglanti zaman asimi (saniye)")
	fs.Parse(args)

	envResults := preflight.CheckEnv()
	envFailed := preflight.Print("env.sh degerleri", envResults)

	sshFailed := false
	if *skipSSH {
		fmt.Println("== SSH erisimi ==")
		fmt.Println("  (--skip-ssh verildi, atlandi)")
		fmt.Println()
	} else if envFailed {
		fmt.Println("== SSH erisimi ==")
		fmt.Println("  (env.sh'de HATA var, SSH testi atlaniyor -- once yukaridaki degerleri duzeltin)")
		fmt.Println()
	} else {
		sshResults := preflight.CheckSSH(sshUser(), *timeout)
		sshFailed = preflight.Print("SSH erisimi", sshResults)
	}

	if envFailed || sshFailed {
		fmt.Println("SONUC: validate-config BASARISIZ -- yukaridaki [HATA] satirlarini duzeltmeden kuruluma/senkrona gecmeyin.")
		os.Exit(1)
	}
	fmt.Println("SONUC: validate-config OK -- env.sh dolu ve tum host'lara SSH ile erisiliyor.")
}

// fanout1, ssh ile TEK bir hosta tek bir alt-komut gonderir (basit trigger).
// RunLive kullanir (Run degil): bu tetiklenen alt-komutlar (db-sync,
// store-sync, config-sync, service-stop, verify-services, fullindex)
// uzun surebilir/yikici olabilir (orn. RESTORE DATABASE) -- kullanicinin
// dakikalarca HICBIR CIKTI gormeden "donmus" sanip Ctrl-C yapmasini
// onlemek icin cikti CANLI akar (bkz. sshrun.RunLive yorumu).
func fanout1(label, host, subcommand string) error {
	t := sshrun.Target{User: sshUser(), Host: host}
	cmd := agentPath() + " " + subcommand
	_, err := sshrun.RunLive(t, cmd)
	if err != nil {
		return fmt.Errorf("[%s] %s uzerinde basarisiz: %w", label, host, err)
	}
	return nil
}

// fanoutAll, ayni alt-komutu BIRDEN FAZLA hosta PARALEL gonderir
// (remote_batch.sh'in "farkli host'lar paralel" kuraliyla ayni).
func fanoutAll(label string, hosts []string, subcommand string) error {
	type result struct {
		host string
		err  error
	}
	ch := make(chan result, len(hosts))
	for _, h := range hosts {
		go func(h string) {
			ch <- result{host: h, err: fanout1(label, h, subcommand)}
		}(h)
	}
	var firstErr error
	for range hosts {
		r := <-ch
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
	}
	return firstErr
}

// agentPath, UZAK host'larda calistirilacak komutun yolunu dondurur.
//
// ONEMLI (daha once gercek ortamda yakalanan hata): burasi BINARY'nin
// kendisini DEGIL, "3dsync-agent-run" SARMALAYICISINI dondurur --
// cunku fanout1/fanoutAll ile baska bir hosta SSH atildiginda, o YENI
// SSH oturumu bu surecin ortam degiskenlerini (mainpath/exppath/host2
// vb.) MIRAS ALMAZ. Sarmalayici, hedef host'ta KENDI config/env.sh
// (+secrets.sh) dosyasini kaynakladıktan SONRA binary'yi cagirir --
// bu sayede her host kendi ortamini kendi env.sh'inden alir. Binary'yi
// DOGRUDAN cagirmak (eski hali), nested SSH cagrilarinda tum env
// degiskenlerinin BOS gelmesine (ve ilk mustEnv() cagrisinda hemen
// hata vermesine) yol aciyordu.
func agentPath() string {
	return mustEnv("mainpath") + "/3dsync-agent-run"
}

// ============================================================
// generate-config / orchestrate (onceden mevcuttu)
// ============================================================

func cmdOrchestrate(args []string) {
	fs := flag.NewFlagSet("orchestrate", flag.ExitOnError)
	clearACL := fs.Bool("clear-acl", false, "fullindex adiminda ACL'leri temizle (geri donusu zor)")
	with3DIndex := fs.Bool("with-3dindex", false, "Adim 12 (3DIndex) calistir")
	fs.Parse(args)

	cfg := orchestrate.FromEnvAndFlags(*clearACL, *with3DIndex)
	if cfg.User == "" || cfg.Host0 == "" || cfg.MainPath == "" {
		fmt.Fprintln(os.Stderr, "HATA: user/host0/mainpath ortam degiskenleri bos -- once config/env.sh kaynaklanmali (source edilmeli).")
		os.Exit(1)
	}

	if err := cfg.Sequence(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdGenerateConfig() {
	cfg := configgen.FromEnv()
	if cfg.MainPath == "" {
		fmt.Fprintln(os.Stderr, "HATA: mainpath ortam degiskeni bos -- once config/env.sh kaynaklanmali (source edilmeli).")
		os.Exit(1)
	}
	if _, err := cfg.Generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ============================================================
// mqlstore / config Config olusturucular
// ============================================================

func mqlCfg() mqlstore.Config {
	return mqlstore.Config{
		ExpPath:   mustEnv("exppath"),
		MainPath:  mustEnv("mainpath"),
		PlmxPath:  os.Getenv("plmxpath"),
		PlmxURL:   os.Getenv("plmxurl"),
		StorePath: os.Getenv("storepath"),
		StoreURL:  os.Getenv("storeurl"),
		ImagePath: os.Getenv("imagepath"),
		ImageURL:  os.Getenv("imageurl"),
	}
}

// configTargetDir, 08_2_modconfig.sh'deki ENOFramework AppInstall/
// Programs hedef dizinidir. "enoconfigdir" env degiskeniyle ACIKCA
// VERILIRSE onceliklidir; verilmezse, bilinen 3DEXPERIENCE surum alt
// dizini (V6R2021x) ile exppath'ten hesaplanir -- bu bir urun surum
// adidir, musteriye ozel bir SIR degil, ama kurulumlar arasinda
// degisebileceginden override edilebilir birakildi.
func configTargetDir() string {
	if d := os.Getenv("enoconfigdir"); d != "" {
		return d
	}
	exppath := mustEnv("exppath")
	return exppath + "/3DSpace/Apps/BusinessProcessServices/V6R2021x/Modules/ENOFramework/AppInstall/Programs"
}

// cmdConfigSync, "config-sync" trigger'idir -- host0'da calisir.
//
// OPTIMIZASYON (remote_batch ile AYNI ilke): 08_1..4 adimlarinin HEPSI
// AYNI hosta (host3) gidiyor -- bu yuzden 5 ayri SSH baglantisi yerine,
// TEK bir SSH ile host3'teki "config-sync-run" alt-komutu tetiklenir;
// o da besini kendi icinde (ayni process, ayri SSH olmadan) sirayla
// calistirir. DIKKAT: bu birlestirme SADECE hedef host AYNI oldugu
// icin guvenli -- adimlar farkli host'lara gidiyorsa (orn.
// service-stop, verify-services) bu desen UYGULANMAZ, cunku tek bir
// SSH oturumu birden fazla hosta baglanamaz. O durumlarda dogru olan
// (ve zaten yapilan) sey: her host kendi SSH'ini alir, farkli host'lar
// PARALEL calisir (bkz. fanoutAll, cmdServiceStop, cmdVerifyServices).
func cmdConfigSync() {
	host3 := mustEnv("host3")
	if err := fanout1("config-sync", host3, "config-sync-run"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// cmdConfigSyncRun, host3 UZERINDE calisir -- 08_1..4 MQL sirasinin
// TUMUNU (beforemod -> generate-config -> modconfig -> aftermod ->
// verify) ayni process icinde, ek SSH baglantisi olmadan yurutur.
func cmdConfigSyncRun() error {
	if err := mqlCfg().BeforeModConfig(); err != nil {
		return fmt.Errorf("config-beforemod basarisiz: %w", err)
	}
	if _, err := configgen.FromEnv().Generate(); err != nil {
		return fmt.Errorf("generate-config basarisiz: %w", err)
	}
	if err := mqlCfg().ModConfig(configTargetDir()); err != nil {
		return fmt.Errorf("config-modconfig basarisiz: %w", err)
	}
	if err := mqlCfg().AfterModConfig(); err != nil {
		return fmt.Errorf("config-aftermod basarisiz: %w", err)
	}
	afterFile := mustEnv("mainpath") + "/SyncResult/config_After.xml"
	if err := mqlstore.VerifyConfig(afterFile, mustEnv("urlconf1"), os.Getenv("urlconf2")); err != nil {
		return fmt.Errorf("config-verify basarisiz: %w", err)
	}
	return nil
}

// ============================================================
// servis durdurma (01_service_stop)
// ============================================================

func cmdServiceStop() {
	host2, host3, host4, host5 := mustEnv("host2"), mustEnv("host3"), mustEnv("host4"), mustEnv("host5")
	host9 := os.Getenv("host9")

	errCh := make(chan error, 5)
	go func() { errCh <- fanout1("service-stop", host2, "service-stop-run apptier") }()
	go func() { errCh <- fanout1("service-stop", host3, "service-stop-run space") }()
	go func() {
		// host4: 3DSpaceIndex -- TomEE degil, cvinit.sh stop (dogrudan).
		t := sshrun.Target{User: sshUser(), Host: host4}
		exppath := mustEnv("exppath")
		cvinit := exppath + "/3DSpaceIndex/linux_a64/cv/data/bin/cvinit.sh"
		out, err := sshrun.Run(t, cvinit+" stop")
		if out != "" {
			fmt.Print(out)
		}
		errCh <- err
	}()
	go func() { errCh <- fanout1("service-stop", host5, "service-stop-run fcs") }()
	if host9 != "" {
		go func() { errCh <- fanout1("service-stop", host9, "service-stop-run fcsremote") }()
		dieOnAny(errCh, 5)
	} else {
		dieOnAny(errCh, 4)
	}
}

func dieOnAny(ch <-chan error, n int) {
	var firstErr error
	for i := 0; i < n; i++ {
		if err := <-ch; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	dieOn(firstErr)
}

// cmdServiceStopRun, "service-stop-run <role>" alt-komutunun gercek
// isi -- bu host UZERINDE (role'un ait oldugu sunucuda) calisir.
func cmdServiceStopRun(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "kullanim: 3dsync-agent service-stop-run <apptier|space|fcs|fcsremote>")
		os.Exit(1)
	}
	exppath := mustEnv("exppath")
	const maxWait = 20 * time.Second

	var fail bool
	switch args[0] {
	case "apptier":
		checks := []struct{ label, env string }{
			{"Comment", "commenttomeepath"},
			{"Dashboard", "dashboardtomeepath"},
			{"Passport", "passporttomeepath"},
			{"FedSearch", "fedsearchtomeepath"},
			{"Swym", "swymtomeepath"},
		}
		for _, c := range checks {
			if err := tomee.StopApp(c.label, mustEnv(c.env), maxWait); err != nil {
				fmt.Fprintln(os.Stderr, err)
				fail = true
			}
		}
		nodeBin := exppath + "/3DNotification/linux_a64/code/command/node"
		if err := tomee.StopNodeProcess("Notification", nodeBin, "3DNotification"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			fail = true
		}
	case "space":
		if err := tomee.StopApp("3DSpace", mustEnv("spacetomeepath"), maxWait); err != nil {
			fmt.Fprintln(os.Stderr, err)
			fail = true
		}
		if err := tomee.StopApp("3DSpaceInternal", mustEnv("internaltomeepath"), maxWait); err != nil {
			fmt.Fprintln(os.Stderr, err)
			fail = true
		}
	case "fcs":
		if err := tomee.StopApp("FCS Central", mustEnv("fcstomeepathcentral"), maxWait); err != nil {
			fmt.Fprintln(os.Stderr, err)
			fail = true
		}
	case "fcsremote":
		if err := tomee.StopApp("FCS Remote", mustEnv("fcstomeepathremote"), maxWait); err != nil {
			fmt.Fprintln(os.Stderr, err)
			fail = true
		}
	default:
		fmt.Fprintf(os.Stderr, "HATA: bilinmeyen rol '%s'\n", args[0])
		os.Exit(1)
	}
	if fail {
		os.Exit(1)
	}
}

// ============================================================
// FCS kopyalama (03_fcs)
// ============================================================

// cmdFCSTrigger, 03_fcs_trigger.sh'in karsiligidir: host0'dan host5'e
// SSH ile "fcs-run" cagrisi yapar (fanout1, canli cikti icin RunLive
// kullanir -- rsync dakikalarca surebilir).
func cmdFCSTrigger() {
	host5 := mustEnv("host5")
	if err := fanout1("fcs-trigger", host5, "fcs-run"); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("HATA: FCS kopyalama basarisiz oldu: %w", err))
		os.Exit(1)
	}
}

// cmdFCSRun, 03_fcs_worker.sh'in karsiligidir -- host5 UZERINDE calisir,
// host7'den (PROD/kaynak) rsync ile ayna-modu kopyalama yapar.
func cmdFCSRun() {
	cfg := fcssync.Config{
		SSHUser:  sshUser(),
		Host7:    mustEnv("host7"),
		Src1:     mustEnv("src1"),
		Dest1:    mustEnv("dest1"),
		MainPath: mustEnv("mainpath"),
	}
	dieOn(cfg.Sync())
}

// ============================================================
// database (04_database / 05_database_restore)
// ============================================================

func dbCfg() dbsync.Config {
	return dbsync.Config{
		SSHUser:     sshUser(),
		Host1:       mustEnv("host1"),
		Host8:       mustEnv("host8"),
		SQLCmdDir:   os.Getenv("sqlcmd"),
		Username:    mustEnv("Username"),
		Password:    mustEnv("password"),
		DBName:      mustEnv("DB_NAME"),
		SQLPath2:    os.Getenv("sqlpath2"),
		Src2:        os.Getenv("src2"),
		Dest2:       os.Getenv("dest2"),
		BackupFile2: os.Getenv("BACKUP_FILE2"),
		BackupFile:  os.Getenv("BACKUP_FILE"),
		SQLData:     os.Getenv("sqldata"),
		SQLLog:      os.Getenv("sqllog"),
	}
}

func cmdDBSync() {
	host1 := mustEnv("host1")
	if err := fanout1("db-sync", host1, "db-backup-run"); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("HATA: DB backup basarisiz oldu, durduruluyor: %w", err))
		os.Exit(1)
	}
	if err := fanout1("db-sync", host1, "db-restore-run"); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("HATA: DB restore basarisiz oldu, durduruluyor: %w", err))
		os.Exit(1)
	}
}

// ============================================================
// servis dogrulama (10_5_verify_services)
// ============================================================

func cmdVerifyServices() {
	host2, host3, host4 := mustEnv("host2"), mustEnv("host3"), mustEnv("host4")
	host5, host6 := mustEnv("host5"), mustEnv("host6")
	host9 := os.Getenv("host9")

	type job struct{ host, args string }
	jobs := []job{
		{host2, "verify-run apptier"},
		{host3, "verify-run space"},
		{host4, "verify-run index"},
		{host5, "verify-run fcs"},
		{host6, "verify-run lb"},
	}
	if host9 != "" {
		jobs = append(jobs, job{host9, "verify-run fcsremote"})
	}

	errCh := make(chan error, len(jobs))
	for _, j := range jobs {
		go func(j job) { errCh <- fanout1("verify-services", j.host, j.args) }(j)
	}
	var firstErr error
	for range jobs {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		fmt.Fprintln(os.Stderr, "HATA: bir veya daha fazla servis beklenen surede ayaga kalkmadi.")
		fmt.Fprintln(os.Stderr, firstErr)
		os.Exit(1)
	}
}

func cmdVerifyRun(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "kullanim: 3dsync-agent verify-run <apptier|space|fcs|fcsremote|index|lb>")
		os.Exit(1)
	}
	exppath := mustEnv("exppath")
	var err error
	switch args[0] {
	case "apptier":
		err = verify.AppTier(mustEnv("commenttomeepath"), mustEnv("dashboardtomeepath"),
			mustEnv("passporttomeepath"), mustEnv("fedsearchtomeepath"), mustEnv("swymtomeepath"))
	case "space":
		err = verify.Space(mustEnv("spacetomeepath"), mustEnv("internaltomeepath"))
	case "fcs":
		err = verify.FCS(mustEnv("fcstomeepathcentral"))
	case "fcsremote":
		err = verify.FCSRemote(mustEnv("fcstomeepathremote"))
	case "index":
		cvinit := exppath + "/3DSpaceIndex/linux_a64/cv/data/bin/cvinit.sh"
		err = verify.Index(cvinit)
	case "lb":
		err = verify.LB(os.Getenv("lbservice"))
	default:
		fmt.Fprintf(os.Stderr, "HATA: bilinmeyen rol '%s'\n", args[0])
		os.Exit(1)
	}
	// DIKKAT: dieOn() KULLANILMIYOR -- asagidaki verify.* fonksiyonlarinin
	// (AppTier/Space/FCS/FCSRemote/Index/LB) HEPSI, basarisiz her kontrolu
	// KENDI ICINDE zaten fmt.Println(err) ile basiyor (bazilari -- AppTier/
	// Space -- TUM basarisiz kontrolleri tek tek basar, sadece SONUNCUSUNU
	// dondurur). dieOn() burada cagrilirsa, o SONUNCU hatayi BIR KEZ DAHA
	// basar (gercek bir ortamda boyle tespit edildi: "FCS Central",
	// "3DSpaceInternal", "Notification" ikiser kez goruldu). Mesaj zaten
	// basildigi icin burada sadece exit code'u ayarliyoruz.
	if err != nil {
		os.Exit(1)
	}
}

// ============================================================
// fullindex (11_fullindex)
// ============================================================

func cmdFullindexTrigger(args []string) {
	fs := flag.NewFlagSet("fullindex", flag.ExitOnError)
	clearACL := fs.Bool("clear-acl", os.Getenv("CLEAR_ACL") == "1", "accesslist'i temizle (geri donusu zor)")
	fs.Parse(args)

	host3 := mustEnv("host3")
	sub := "fullindex-run"
	if *clearACL {
		sub = "fullindex-run --clear-acl"
	}
	dieOn(fanout1("fullindex", host3, sub))
}

func cmdFullindexRun(args []string) {
	fs := flag.NewFlagSet("fullindex-run", flag.ExitOnError)
	clearACL := fs.Bool("clear-acl", os.Getenv("CLEAR_ACL") == "1", "accesslist'i temizle (geri donusu zor)")
	fs.Parse(args)

	cfg := fullindex.Config{
		MQL:      mqlCfg(),
		SSHUser:  sshUser(),
		Host4:    mustEnv("host4"),
		ClearACL: *clearACL,
	}
	swapBase := os.Getenv("swap_base_dir")
	if swapBase == "" {
		swapBase = "/db"
	}
	dieOn(cfg.Run(swapBase))
}

// ============================================================
// 3dindex (12_3dindex)
// ============================================================

func threedindexCfg() threedindex.Config {
	return threedindex.Config{
		SSHUser:  sshUser(),
		Host4:    mustEnv("host4"),
		ExpPath:  mustEnv("exppath"),
		MainPath: mustEnv("mainpath"),
	}
}

// ============================================================
// wait-for-hosts (lib/wait_for_hosts.sh)
// ============================================================

func cmdWaitForHosts(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "kullanim: 3dsync-agent wait-for-hosts <host1> [host2 ...]")
		os.Exit(1)
	}
	maxWait := 300 * time.Second
	interval := 10 * time.Second
	if v := os.Getenv("WAIT_FOR_HOSTS_MAX"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			maxWait = d
		}
	}
	if v := os.Getenv("WAIT_FOR_HOSTS_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			interval = d
		}
	}
	dieOn(waithosts.Wait(sshUser(), args, maxWait, interval))
}

// cmdInitConfig, "init-config" alt-komutunun gerceklestirmesidir -- bkz.
// internal/initconfig paketi. Bu komut, env.sh SABLONUNDAKI "CHANGE_ME_"
// yer tutucularini interaktif sorularla VEYA bir "--answers" dosyasiyla
// doldurur; hicbir host'a baglanmaz, hicbir senkron adimi CALISTIRMAZ.
func cmdInitConfig(args []string) {
	fs := flag.NewFlagSet("init-config", flag.ExitOnError)
	template := fs.String("template", "config/env.sh", "doldurulacak sablon env.sh dosyasinin yolu")
	output := fs.String("output", "", "cikis dosyasi yolu (bos ise --template ile AYNI -- yerinde guncellenir, eski hali .bak olarak yedeklenir)")
	answers := fs.String("answers", "", "KEY=VALUE formatinda bir cevap dosyasi (cok sayida kurulumda sorulari scriptlemek icin)")
	nonInteractive := fs.Bool("non-interactive", false, "answers dosyasinda olmayan HER degiskeni hata sayar, hicbir soru sormaz")
	fs.Parse(args)

	err := initconfig.Run(initconfig.Options{
		TemplatePath:   *template,
		OutputPath:     *output,
		AnswersPath:    *answers,
		NonInteractive: *nonInteractive,
	})
	dieOn(err)
}

// ============================================================
// log temizligi + servis baslatma (10_clear_logs_and_start_services)
// ============================================================

// cmdClearLogsStartServices, host0 UZERINDE calisir -- bkz. internal/postsync.
// "exppath" ve "host9"/"fcstomeepathremote" DISINDA tum degerler o
// rolun bu musteride var oldugunu varsayar; bir rol yoksa ilgili
// env degiskeni (orn. "fedsearchtomeepath") bos birakilabilir --
// postsync.BuildJobs() bos yollari otomatik atlar.
func cmdClearLogsStartServices() {
	h := postsync.Hosts{
		User: sshUser(),

		Host2: mustEnv("host2"),
		Host3: mustEnv("host3"),
		Host4: os.Getenv("host4"),
		Host5: os.Getenv("host5"),
		Host6: os.Getenv("host6"),
		Host9: os.Getenv("host9"),

		CommentTomeePath:   os.Getenv("commenttomeepath"),
		DashboardTomeePath: os.Getenv("dashboardtomeepath"),
		PassportTomeePath:  os.Getenv("passporttomeepath"),
		FedsearchTomeePath: os.Getenv("fedsearchtomeepath"),
		SwymTomeePath:      os.Getenv("swymtomeepath"),
		SpaceTomeePath:     os.Getenv("spacetomeepath"),
		InternalTomeePath:  os.Getenv("internaltomeepath"),
		FCSCentralPath:     os.Getenv("fcstomeepathcentral"),
		FCSRemotePath:      os.Getenv("fcstomeepathremote"),
		ExpPath:            os.Getenv("exppath"),
	}
	dieOn(h.ClearLogsAndStartServices())
}

// ============================================================
// izin hazirlama (eski setup/01_prepare_permissions.sh + setup/02_deploy.sh)
// ============================================================

// cmdPreparePermissions, eski setup/01_prepare_permissions.sh +
// setup/02_deploy.sh'nin KALAN gecerli kismidir.
//
// NEDEN KUCULDU: 01'in dos2unix + chmod +x islevi ve 02'nin "bash script
// agacini TUM host'lara scp'leme" islevi, artik install.sh tarafindan
// FARKLI (ve daha basit) bir modelle zaten yapiliyor -- her host'a tek
// bir binary + wrapper + env.sh/secrets.sh kopyalaniyor, chmod +x ve
// secrets.sh icin chmod 600 KURULUM SIRASINDA (install.sh icinde) bir
// kere uygulaniyor (bkz. install.sh). Dolayisiyla bu iki eski script HER
// SENKRON CALISTIRMASINDA bash dosyalarini yeniden dagitmaya calisirdi --
// bu artik hem gereksiz hem de YANLIS (o dosyalar artik hedef hostlarda
// olmayabilir). KALAN tek gercek is: SQL Server'in VE MQL/config
// islemlerinin ihtiyac duydugu calisma-zamani izinleri (02_deploy.sh'nin
// sonundaki "Yetki verme" blogu) -- bu, HER senkrondan once hala
// calistirilmasi gereken, dosya dagitimiyla ILGISIZ bir adimdir.
func cmdPreparePermissions() {
	host1 := mustEnv("host1")
	host3 := mustEnv("host3")
	user := sshUser()

	type job struct{ host, cmd string }
	var jobs []job
	add := func(host, path, mode string) {
		if path == "" {
			return
		}
		jobs = append(jobs, job{host, fmt.Sprintf("sudo chmod %s -R %s", mode, path)})
	}

	// database (host1): sqlpath/sqldata/sqllog HER ZAMAN, sqlindex
	// SADECE tanimliysa (bazi ortamlarda -- orn. DEV -- gecerli/kullanilmiyor).
	add(host1, os.Getenv("sqlpath"), "777")
	add(host1, os.Getenv("sqldata"), "777")
	add(host1, os.Getenv("sqllog"), "777")
	add(host1, os.Getenv("sqlindex"), "777")

	// space (host3): config.xml uretim/dogrulama icin config/ dizini.
	// NOT: orijinalde "$mainpath/steps" de vardi -- artik bash script
	// agaci dagitilmadigi icin o kisim KALDIRILDI (atilacak bir
	// 'steps/' dizini yok).
	add(host3, mustEnv("mainpath")+"/config", "775")

	if len(jobs) == 0 {
		fmt.Println("[prepare-permissions] izin verilecek hicbir yol tanimli degil (sqlpath/sqldata/sqllog/sqlindex hepsi bos) -- atlaniyor.")
		return
	}

	var fail bool
	for _, j := range jobs {
		t := sshrun.Target{User: user, Host: j.host}
		out, err := sshrun.Run(t, j.cmd)
		if out != "" {
			fmt.Print(out)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[prepare-permissions] UYARI: %s@%s uzerinde '%s' basarisiz: %v\n", user, j.host, j.cmd, err)
			fail = true
		}
	}
	if fail {
		os.Exit(1)
	}
	fmt.Println("[prepare-permissions] tamamlandi.")
}

func usage() {
	fmt.Fprintln(os.Stderr, `3dsync-agent <komut> [bayraklar/argumanlar]

Ana komut:
  orchestrate [--clear-acl] [--with-3dindex]   Tam senkronizasyon akisini calistirir (host0)
  validate-config [--skip-ssh] [--ssh-timeout N]   env.sh'i ve host SSH erisimini dogrular, HICBIR SEY DEGISTIRMEZ
  init-config [--template f] [--output f] [--answers f] [--non-interactive]
                                Sablon env.sh'deki CHANGE_ME_ yer tutucularini doldurur (soru-cevapla veya --answers ile)

Trigger komutlari (host0'da calisir, hedef host'lara SSH ile fan-out yapar):
  service-stop                 Tum TomEE/cvinit servislerini durdurur
  task-stop                    host3+host4'te crond durdurur
  fcs-trigger                   FCS kopyalamayi (rsync ayna) host5'te tetikler
  store-sync                   06_store MQL sirasini host3'te calistirir
  db-sync                      DB backup+restore'u host1'de calistirir
  config-sync                  08_config MQL+dogrulama sirasini host3'te calistirir
  clear-logs-start-services     Log temizligi + tum servisleri baslatir (10)
  verify-services              Tum servislerin ayakta oldugunu dogrular
  fullindex [--clear-acl]      Fullindex sirasini host3'te tetikler
  3dindex-trigger               3DIndex'i host4'te tetikler
  restart-scheduler             host3+host4'te crond yeniden baslatir
  prepare-permissions           DB + config dizinlerinde calisma-zamani izinlerini ayarlar
  wait-for-hosts <host...>      SSH geri gelene kadar bekler (reboot sonrasi)

"-run" komutlari (ilgili hedef host UZERINDE calisir, genelde trigger
komutlari tarafindan ssh ile cagrilir, elle de calistirilabilir):
  generate-config, store-sync-run, config-sync-run (beforemod/modconfig/
  aftermod/verify'in TUMUNU TEK SSH'de calistirir), service-stop-run <rol>,
  verify-run <rol>, crond-stop-run, crond-restart-run, db-backup-run,
  db-restore-run, fullindex-run [--clear-acl], threedindex-run, fcs-run

  version                        Surum bilgisini gosterir`)
}
