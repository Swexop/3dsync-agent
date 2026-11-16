// Package postsync, steps/10_clear_logs_and_start_services.sh dosyasinin
// Go karsiligidir. Bu adim host0 UZERINDE calisir ve remotebatch
// (lib/remote_batch.sh'in Go karsiligi) araciligiyla diger host'lara
// SSH fan-out'u yapar -- host basina komutlar SIRAYLA (temizlik ->
// baslatma bagimliligi bozulmasin diye), farkli host'lar PARALEL.
package postsync

import (
	"khantech.com/3dsync-agent/internal/remotebatch"
)

// Hosts, bu adimin ihtiyac duydugu TUM host/yol degerlerini tasir.
// host9/FCSRemotePath opsiyoneldir (bos ise o host icin islem
// kuyruklanmaz). Herhangi bir tomeePath alani bos ise (bu musteride o
// rol yoksa), o role ait temizlik/baslatma komutlari sessizce atlanir.
type Hosts struct {
	User string

	Host2 string // AppTier (Comment/Dashboard/Passport/FedSearch/Swym/Notification)
	Host3 string // Space + Space Internal
	Host4 string // 3DSpaceIndex
	Host5 string // FCS Central
	Host6 string // Load Balancer (HAProxy/Apache/nginx)
	Host9 string // FCS Remote (opsiyonel)

	CommentTomeePath   string
	DashboardTomeePath string
	PassportTomeePath  string
	FedsearchTomeePath string
	SwymTomeePath      string
	SpaceTomeePath     string
	InternalTomeePath  string
	FCSCentralPath     string
	FCSRemotePath      string // opsiyonel (Host9 ile birlikte bos olabilir)
	ExpPath            string
}

// Job, tek bir (host, komut) kuyruk girdisidir -- remotebatch.Batch.Queue
// cagrisinin SAF (I/O'suz) karsiligi.
type Job struct {
	Host string
	Cmd  string
}

// BuildJobs, 10_clear_logs_and_start_services.sh'in TAM sirasini SAF
// (yan etkisiz, kolayca test edilebilir) bir is listesine cevirir: once
// her host icin logs/temp/work temizligi, sonra ayni hostlarin
// servislerini baslatma -- liste sirasi, remotebatch'in host-ici sirayi
// koruyan davranisiyla birlikte orijinal bagimliligi (temizlik HER ZAMAN
// baslatmadan once) garanti eder.
func BuildJobs(h Hosts) []Job {
	var jobs []Job
	add := func(host, cmd string) {
		if host == "" || cmd == "" {
			return
		}
		jobs = append(jobs, Job{Host: host, Cmd: cmd})
	}
	cleanup := func(host, tomeePath string) {
		if tomeePath == "" {
			return
		}
		add(host, "rm -rf "+tomeePath+"/logs/*")
		add(host, "rm -rf "+tomeePath+"/temp/*")
		add(host, "rm -rf "+tomeePath+"/work/*")
	}
	startup := func(host, tomeePath string) {
		if tomeePath == "" {
			return
		}
		add(host, tomeePath+"/bin/startup.sh")
	}

	// --- temizlik ---
	cleanup(h.Host2, h.CommentTomeePath)
	cleanup(h.Host2, h.PassportTomeePath)
	cleanup(h.Host3, h.SpaceTomeePath)
	cleanup(h.Host3, h.InternalTomeePath)
	if h.ExpPath != "" {
		add(h.Host3, "rm -rf "+h.ExpPath+"/3DSpace/logs/*")
		add(h.Host3, "rm -rf "+h.ExpPath+"/3DSpace/logsCAS/*")
	}
	cleanup(h.Host2, h.DashboardTomeePath)
	cleanup(h.Host2, h.SwymTomeePath)
	cleanup(h.Host5, h.FCSCentralPath)
	cleanup(h.Host2, h.FedsearchTomeePath)

	// --- baslatma ---
	startup(h.Host2, h.CommentTomeePath)
	startup(h.Host2, h.DashboardTomeePath)
	startup(h.Host2, h.PassportTomeePath)
	startup(h.Host2, h.FedsearchTomeePath)
	startup(h.Host2, h.SwymTomeePath)
	if h.ExpPath != "" {
		add(h.Host2, h.ExpPath+"/3DNotification/linux_a64/code/command/node start")
	}
	startup(h.Host3, h.SpaceTomeePath)
	startup(h.Host3, h.InternalTomeePath)
	if h.ExpPath != "" {
		add(h.Host4, h.ExpPath+"/3DSpaceIndex/linux_a64/cv/data/bin/cvinit.sh start")
	}
	startup(h.Host5, h.FCSCentralPath)
	if h.Host9 != "" {
		cleanup(h.Host9, h.FCSRemotePath)
		startup(h.Host9, h.FCSRemotePath)
	}

	// LB reload -- hangi yazilimin kurulu oldugu bu hosttan OTOMATIK
	// TESPIT edilir (verify.LB() ile AYNI ruh): haproxy yoksa hata
	// SAYILMAZ, sadece bilgilendirici bir not basilir.
	if h.Host6 != "" {
		add(h.Host6, lbReloadCmd)
	}

	return jobs
}

// ClearLogsAndStartServices, BuildJobs() cikisini gercek bir
// remotebatch.Batch uzerinden SSH ile calistirir (I/O'lu kisim, bilerek
// cok kucuk tutuldu -- asil mantik yukarida, saf halde).
func (h Hosts) ClearLogsAndStartServices() error {
	b := remotebatch.New(h.User)
	for _, j := range BuildJobs(h) {
		b.Queue(j.Host, j.Cmd)
	}
	return b.Flush()
}

// lbReloadCmd, orijinal 10_clear_logs_and_start_services.sh'deki HAProxy
// reload mantiginin birebir karsiligidir: once config syntax kontrolu
// (basarisizsa reload HIC denenmez -- bozuk config asla devreye
// alinmaz), sonra "reload" (aktif baglantilari kesmeden config'i
// tazeler). "command not found" (haproxy hic kurulu degilse, Apache/
// nginx/httpd kullanilan bir ortamda) HATA SAYILMAZ, sadece bilgilendirici
// bir not basilir -- verify.LB()'nin otomatik-tespit mantigiyla AYNI.
const lbReloadCmd = `if command -v haproxy >/dev/null 2>&1; then ` +
	`sudo haproxy -c -f /etc/haproxy/haproxy.cfg && ` +
	`{ sudo service haproxy reload || echo '[UYARI] HAProxy reload basarisiz oldu -- HAProxy CALISMAYA DEVAM EDIYOR, sadece yeni config henuz yuklenmedi.'; }; ` +
	`else echo '[NOT] haproxy komutu bulunamadi -- bu host uzerinde LB yazilimi olarak HAProxy kullanilmiyor olabilir. Adim atlaniyor, senkron DURDURULMUYOR.'; fi`
