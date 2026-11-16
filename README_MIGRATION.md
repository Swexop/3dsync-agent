# 3dsync-agent — Teknik Koruma Göç Planı (Bash → Go)

Bu klasör, AutoSync/3DSync'in bash script'lerindeki "asıl değer taşıyan"
mantığı (adım sıralaması/bağımlılıkları, config dönüşüm kuralları) tek bir
derlenmiş Go binary'sine (`3dsync-agent`) taşımak için başlatılan bir
iskelettir. **Hiçbir mevcut dosya silinmedi/değiştirilmedi** — bu hibrit,
kademeli bir geçiştir.

## Neden bu ikisiyle başladık?

Repo'daki ~50 script içinde en çok "operasyonel know-how" taşıyanlar:

1. **`main/run_full_sync.sh`** — hangi adımın hangisinden önce/sonra/paralel
   çalışması gerektiği (örn. FCS kopyalama DB restore ile paralel ama
   fullindex'ten önce bitmiş olmalı). Bu, PLM ortamlarıyla çalışılarak
   öğrenilmiş tecrübedir — kopyalanması en kolay ama **yeniden üretilmesi en
   zor** kısım budur.
2. **`steps/08_config/08_1_5_generate_config.sh`** — config.xml'in hangi
   kuralla (urlconf1→urlconf2, urlconf3→urlconf4) üretildiği.

Bunlar artık `internal/orchestrate/orchestrate.go` ve
`internal/configgen/configgen.go` içinde, Go kodu olarak duruyor ve
`go build` ile tek bir ELF binary'ye derleniyor.

## Dürüst sınır

`strings 3dsync-agent` çalıştırılırsa, log mesajları ve değişken isimleri
(`urlconf1`, `mainpath` gibi) yine de görülebilir — bunlar string literal
olarak binary'ye gömülür. **Ama kontrol akışının kendisi (hangi sırayla,
hangi koşulda, ne zaman paralel) makine koduna derlenmiştir, düz metin
olarak yoktur.** Bu "okunamaz" değil, "okumak için düz `cat`/`strings`
yerine ciddi tersine mühendislik gerektirir" anlamına gelir — amaç da
zaten buydu (bkz. önceki mesajdaki Seviye 1/2/3 ayrımı).

## Mimari (hibrit geçiş)

```
Kontrol makinesi (ör. KhanTech ofisi)
  $ 3dsync-agent orchestrate --with-3dindex
        │
        │ ssh user@host0 "<legacy script yolu>"     (henüz migrate edilmemiş adımlar)
        │ ssh user@host0 "/opt/3dsync/3dsync-agent generate-config"  (migrate edilmiş adım)
        ▼
  host0 / host3 / ... (müşteri sunucuları)
        - config/env.sh + config/secrets.sh DEĞİŞMEDİ, hâlâ bash, hâlâ orada
        - 3dsync-agent binary'si bu dosyaları env olarak miras alır
          (wrapper: ". config/env.sh && exec ./3dsync-agent "$@"")
```

Her sunucuya artık tek bir `3dsync-agent` binary'si deploy edilir (cross-compile
ile o sunucunun OS/mimarisine göre). Henüz migrate edilmemiş adımlar için
`setup/02_deploy.sh` ile eskisi gibi ilgili `.sh` dosyaları da deploy edilmeye
devam eder — göç tamamlanana kadar ikisi bir arada yaşar.

## Hızlı kurulum (tek komut)

```bash
./install.sh /path/to/config/env.sh [/path/to/config/secrets.sh]
```

Bu script: (1) Go kuruluysa binary'yi derler, (2) `config/env.sh` +
`config/secrets.sh`'i otomatik kaynaklayan bir `3dsync-agent-run`
sarmalayıcısı üretir, (3) `env.sh`'deki `host0`..`host9`'un HEPSİNE
(tekrarlar elenerek) binary + sarmalayıcıyı `scp` ile kopyalar ve
`chmod +x` yapar, (4) `host0`'da (yoksa) bir SSH anahtarı oluşturup
**diğer tüm host'ların `authorized_keys`'ine ekler**. `config/env.sh`/
`secrets.sh`'in içeriğine dokunmaz, hiçbir değeri ekrana yazdırmaz.

**4. adım neden gerekli?** `verify-services`, `service-stop`,
`store-sync` gibi "trigger" komutları `host0`'da çalışıp KENDİ İÇİNDEN
diğer host'lara SSH ile fan-out yapıyor — bu çağrılar bir script
içinden tetiklendiği için terminal/TTY yok, parola SORAMAZLAR. Bu
adım olmadan hepsi "Permission denied" ile düşer (gerçek bir ortamda
tam böyle oldu). Eski bash'teki `remote_batch`/`remote_queue` mantığı
da zaten şifresiz SSH'e bağımlıydı — bu yeni bir gereksinim değil,
ilk kez otomatik tetiklenince fark ediliyor. Bu adımı atlamak isterseniz
`--no-ssh-keys` bayrağını ekleyin: `./install.sh env.sh secrets.sh --no-ssh-keys`.

Kurulum bittikten sonra, önce SADECE-OKUYAN komutlarla test edin:

```bash
ssh $user@<host> "$mainpath/3dsync-agent-run version"
ssh $user@<host> "$mainpath/3dsync-agent-run verify-services"
```

(`$mainpath`/`$user` değerlerini kendi `env.sh`'inizden alın.) Bundan
sonraki adımlar (izole test → QA'da tam `orchestrate` → prod) için
aşağıdaki "Deploy" ve göç-durumu bölümlerine bakın.

## Elle derleme

```bash
cd 3dsync-agent
go build -ldflags="-s -w" -o 3dsync-agent ./cmd/3dsync-agent
# Hedef sunucu farklı OS/mimaride ise, örn.:
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o 3dsync-agent ./cmd/3dsync-agent
```

`-ldflags="-s -w"` sembol tablosunu ve debug bilgisini atar — tersine
mühendisliği biraz daha zorlaştırır, tam koruma sağlamaz.

## Test

```bash
go test ./...
```

`internal/configgen/configgen_test.go`, orijinal `08_1_5_generate_config.sh`
ile aynı 3 senaryoyu (tek değişim, değişim yok, kaynak dosya eksik) doğrular.

## Deploy (bir sonraki adım — henüz otomatikleştirilmedi)

`setup/02_deploy.sh` içine, ilgili host'lara artık `.sh` dosyaları yanında
`3dsync-agent` binary'sinin de `scp` ile kopyalanması eklenmeli:

```bash
scp 3dsync-agent $user@$host3:$mainpath/
ssh $user@$host3 "chmod +x $mainpath/3dsync-agent"
```

Ve `main/run_full_sync.sh` → `3dsync-agent orchestrate` ile değiştirilmeli
(ya da ikisi paralel tutulup kademeli geçilmeli).

## Göç durumu: TAMAMLANDI (06_store, 08_config, 10_5_verify_services, 01_service_stop, 02_task_stop, 13_restart_scheduler, 04_database, 05_database_restore, 11_fullindex, 12_3dindex)

Kullanıcının "hepsini yapalım" talimatıyla, önceliklendirilmiş listedeki
TÜM script'ler Go'ya taşındı. Her biri kendi `internal/<paket>` altında:

| Paket | Taşıdığı script(ler) | Not |
|---|---|---|
| `mqlstore` | `06_store/06_*.sh`, `08_config/08_1..4_*.sh` | MQL path/URL/searchindex komut sırası; `08_4` kritik bug-fix (naked-URL kontrolü) korundu |
| `dbsync` | `04_database_worker.sh`, `05_restore_worker.sh` | `RESTORE FILELISTONLY`→dinamik `MOVE` mantığı (`BuildMoveClauses`) birim testli |
| `tomee` | `lib/stop_tomee.sh`, `01_stop_*.sh` | nazikçe shutdown.sh → pgrep doğrulama → zorla kill -9 |
| `verify` | `10_5_verify_services/verify_*.sh` | orijinal timeout/interval değerleri (120s/5s, 180s/5s, 60s/3s) birebir |
| `swapmon` | `lib/ensure_swap.sh`, `11_0_swap_monitor_start/11_9_swap_monitor_stop` | arka plan izleyici artık PID dosyası değil, `context.Context` ile yönetilen goroutine |
| `fullindex` | `11_fullindex/11_*.sh` | swap-monitor + validate + (opsiyonel clearacl) + clearindex + stopindex + fullindex_start (host4 cvinit 50s/50s/100s uyku dahil) + aftervalidate + cleanup — hepsi tek `Run()` |
| `threedindex` | `12_3dindex/*.sh` | **BİLEREK KORUNAN BUG**: `12_3dindex_run.sh`'nin sonundaki komutsuz `timeout 3600` no-op'u aynen taşındı (müşteri onayı bekleniyor, sessizce "düzeltilmedi") |
| `waithosts` | `lib/wait_for_hosts.sh` | reboot sonrası SSH-geri-gelme kontrolü, orkestratörden direkt |
| `crond` | `02_task_stop.sh`, `13_restart_scheduler.sh` | host3+host4'te crond durdur/başlat |
| `remotebatch` | `lib/remote_batch.sh` | aynı host sıralı, farklı host paralel kuyruk/flush |

`cmd/3dsync-agent/main.go`, her paketi "trigger" (host0'da çalışıp hedef
host'lara SSH fan-out yapan) ve "-run" (hedef host ÜZERİNDE doğrudan
çalışan) alt-komutlarına bağladı — bkz. dosyanın başındaki yorum ve
`usage()` çıktısı. `internal/orchestrate/orchestrate.go`'daki
`Sequence()`, artık SADECE şunlar için `legacyStep(...)` (eski bash
yoluna SSH) kullanıyor:

- `setup/01_prepare_permissions.sh`, `setup/02_deploy.sh` (Öncelik 3 — genel SSH/sudo kurulum glue, koruma değeri düşük)
- `steps/03_fcs/03_fcs_trigger.sh` (rsync/scp glue, generic)
- `steps/10_clear_logs_and_start_services.sh` (generic start glue)
- `steps/07_location/*.sh` (TOGG'da zaten devre dışı, çağrılmıyor)

Bunların hepsi, repodaki ~50 script içinde EN AZ "operasyonel know-how"
taşıyan kısımlardı (bkz. önceki önceliklendirme tablosu) — rakibin kendi
başına kolayca yazabileceği, kopyalanmasının asıl değeri taşımadığı
glue kod. İsteniyorsa bunlar da aynı `agentStep(...)` desenine taşınabilir,
ama göç planının asıl hedefine (yüksek-değerli mantığı derlenmiş koda
almak) artık ulaşıldı.

## Gerçek ortamda doğrulama (canlı sunucu, 2026-11-10)

Müşterinin kendi sunucusuna (`KHT2025x`, host0=`10.1.1.34`) `install.sh` ile
gerçek dağıtım yapılıp, adım adım canlı doğrulama gerçekleştirildi. Bu
sırada üç gerçek hata bulunup düzeltildi (hepsi bu depoya işlendi):

1. **SSH anahtar eksikliği** — "trigger" komutları (`verify-services` vb.)
   host0'dan diğer host'lara TTY'siz SSH fan-out yapıyor, parola
   SORAMIYOR. `install.sh`'e otomatik, idempotent SSH-key dağıtımı eklendi.
2. **Self-SSH güveni** — host0'daki bir env.sh rolü (`hostN`) host0'un
   KENDI IP'sine esitse, host0 kendi public key'ine de guvenmeli (ayri bir
   makine degil diye otomatik sayilmiyor). `install.sh`'deki key-dagitim
   dongusu `h == host0` durumunu artik ATLAMIYOR.
3. **`agentPath()` sarmalayici bypass'i (gercek KOD bug'u)** — `main.go`
   nested-SSH fan-out cagrilarinda binary'yi DOGRUDAN (`3dsync-agent`)
   cagiriyordu; bu, nested SSH oturumunun env.sh'i HIC kaynaklamamasi
   (bos ortam degiskenleri, `exppath` bos hatasi) anlamina geliyordu.
   Duzeltme: `agentPath()` artik sarmalayiciyi (`3dsync-agent-run`)
   donduruyor. `install.sh` de artik `config/env.sh`+`secrets.sh`'i HER
   host'a (sadece host0'a degil) kopyaliyor.

Ardindan `config-sync` zincirinin TUMU (`config-beforemod-run` →
`generate-config` → `config-modconfig-run` → `config-aftermod-run` →
`config-verify-run`) gercek MQL (`mql -c`) ile, gercek
3DEXPERIENCE/searchindex konfigurasyonuna karsi, TEK TEK calistirilip
dogrulandi:

- `config-beforemod-run`: gercek 5.9MB `config_Before.xml` uretti
  (host `3dspacefts.25xkht.com.tr` iceren gercek veri).
- `generate-config`: `urlconf1`→`urlconf2` (`3dspacefts`→`3dspaceftsqa`)
  donusumunu gercek veri uzerinde dogru uyguladi.
- `config-modconfig-run`: uretilen `config.xml`'i gercek hedef dizine
  kopyalayip `mql`'e `set system searchindex file ... force` verdi.
- `config-aftermod-run` + `config-verify-run`: guncellenmis config'i
  tekrar okuyup hedef URL'nin var, eski URL'nin (naked kalinti olarak)
  YOK oldugunu dogruladi — **`OK config.xml icinde hedef URL
  dogrulandi, degistirilmemis kalinti yok.`**

Bu, MQL baglantisi + config donusum mantigi + binary dagitim/wrapper/
SSH-key/env-kaynaklama mekanigi'nin TUMUNUN, sahte dosya sistemiyle
degil GERCEK bir 3DEXPERIENCE ortaminda uctan uca calistigini kanitlayan
ilk doğrulamadır. `verify-services` de ayni oturumda calistirilip, LB
servisinin HAProxy'ye sabit olmadigini (host6'da Apache de olabilir)
gosteren gercek geri bildirimle `verify.LB()` otomatik-tespit+`lbservice`
env-override'ina guncellendi.

Ardindan `store-sync` (host3'teki 06_store MQL zinciri, 7 adim) VE
`db-sync` (host1'de gercek BACKUP DATABASE -> RESTORE FILELISTONLY ->
dinamik MOVE -> offline -> drop -> RESTORE DATABASE, 157969 sayfa,
spacedb) de gercek ortamda TAM olarak dogrulandi -- ikisi de EXIT=0,
sonrasinda `spacedb` state_desc=ONLINE olarak teyit edildi. `store-sync`
icin aftermod ciktisi env.sh'deki hedef path/url degerleriyle BIREBIR
karsilastirilip dogrulandi. `db-sync` sirasinda, dorduncu gercek hata
bulunup duzeltildi:

4. **`fanout1()`'in sessiz buffer'lamasi (UX/guvenlik riski)** --
   `sshrun.Run()`, uzak komutun TUM ciktisini SSH oturumu TAMAMEN
   bitene kadar hic yazdirmadan topluyordu. Gercek bir ortamda,
   `db-restore-run` (3.8GB'lik bir RESTORE DATABASE icerir) sirasinda
   bu, kullaniciya dakikalarca HICBIR CIKTI gostermedi -- surecin
   "donmus" sanilip, ORTASINDA (offline/drop/restore adimlarindan
   birinde) Ctrl-C yapilmasina yol acabilecek bir risk olusturdu.
   Duzeltme: yeni `sshrun.RunLive()`, cikisi ANINDA (satir satir) da
   bu surecin kendi stdout'una akitiyor; `fanout1()` artik Run()
   yerine bunu kullaniyor (db-sync, store-sync, config-sync,
   service-stop, verify-services, fullindex tetikleyicilerinin
   HEPSI artik canli cikti veriyor).

`verify-services` tekrar calistirilinca (yeni RunLive binary'siyle),
canli cikti dogrulandi AMA kucuk bir cikti-tekrari bulundu: AppTier/
Space/FCS kendi icinde basarisiz her kontrolu zaten `fmt.Println(err)`
ile basiyordu, `cmdVerifyRun`'daki `dieOn(err)` ayni (sondaki) hatayi
BIR KEZ DAHA basiyordu ("FCS Central"/"3DSpaceInternal"/"Notification"
ikiser kez gorundu). Duzeltme: `Index()`/`LB()` de tutarlilik icin
kendi icinde basacak sekilde guncellendi, `cmdVerifyRun` artik `dieOn()`
yerine sessizce sadece exit code ayarliyor -- her hata artik TAM OLARAK
BIR KEZ basiliyor.

## Yuksek riskli adimlarin gercek ortamda dogrulanmasi (2026-11-11)

Sunucunun VM-snapshot yedegi oldugu ve bilerek "bozulabilir" bir test
ortami oldugu musteri tarafindan teyit edildikten sonra, kalan iki
yuksek riskli adim da denendi:

- **`3dindex-trigger`**: calisirken BES'INCI gercek hata bulundu --
  `threedindex.Trigger()`, host4'teki nested-SSH cagrisinda (`agentPath()`
  ile AYNI bug sinifi) ham binary'yi (`3dsync-agent`) cagiriyordu,
  sarmalayiciyi (`3dsync-agent-run`) degil; bu da env.sh'nin hic
  kaynaklanmamasina (`'user' ortam degiskeni bos` hatasi) yol aciyordu.
  Duzeltildi. Testler sirasinda, `mv`/`rm` adimlarinin sirasi
  karistirilinca (kullanici talimati atlayinca) test ortamindaki
  `3DIndexes`/`3DIndexes_old` veri icerigi kazayla silindi -- bu bir KOD
  hatasi degildi (VM yedegi oldugu icin musteri tarafindan kabul
  edilebilir bulundu), ama gercek bir mimari gercegi ORTAYA COKARDI:
  `BBDAdminPlayer`/crawler, BOS bir `3DIndexes` dizininden calismiyor --
  `BBDMonitor/BBDCrawlerReport.xml` gibi onceden var olmasi gereken bir
  alt yapiya ihtiyac duyuyor ("Cannot launch crawler!!"). Bu, scriptin
  /kodun bir eksikligi degil -- 3DIndex'in kendi mimarisinin bir
  geregi; `3dindex-trigger`'in KENDI mantigi (dosya tasima + sarmalayici
  araciligiyla tetikleme) artik dogru calisiyor, sadece gercek indeks
  icerigi test sirasinda kaybedildigi icin crawler calisamadi.
- **`fullindex`** (host3, `--clear-acl` OLMADAN -- varsayilan guvenli mod):
  **TAM olarak basariyla tamamlandi** -- `validate → clearindex →
  stopindex → fullindex_start (host4: cvinit stop/temizle/start + MQL
  "start searchindex mode full") → 300sn bekleme → aftervalidate`,
  hepsi `EXIT=0`. `ensure_swap` arka plan izleyicisi de dogru calisti
  (bos bellek esigin uzerinde oldugu icin yeni swap eklemedi, beklenen
  davranis). Bu, **projenin EN KRITIK/EN RISKLI adiminin** gercek
  ortamda ilk basariyla dogrulanmasidir.
- Bu sirada ALTINCI gercek (kucuk) hata bulundu: `swapmon.Cleanup()`,
  `swap_base_dir` hic set edilmemis (varsayilan `/db`, diskte olmayan)
  bir ortamda gereksiz bir HATA donduruyordu; orijinal bash'teki glob
  (`for f in $base_dir/swap_autosync_*.img`) dizin yoksa SESSIZCE
  "temizlenecek bir sey yok" diyordu. Duzeltildi: `Cleanup()` artik
  `os.IsNotExist` durumunda da ayni sekilde sessiz/bilgilendirici
  davraniyor, hata donmuyor.

**`--clear-acl` ile `fullindex` de dogrulandi (2026-11-16):** birkac gun
sonraki bir oturumda denendiginde, VM'in bu arada yeniden baslatilmis
olmasi iki gercek (kod-disi, ortam) sorunu ortaya cikardi: (1) SQL
Server yeni baslamisti, ilk denemede "Login timeout expired" verdi --
birkac dakika beklendi, cozuldu; (2) `validate searchindex` basarisiz
oldu cunku arama indeksi konsolidasyon sunucusunun hostname'i
(`3dspaceftsqa.25xkht.com.tr`, config-sync'in DOGRU sekilde uyguladigi
hedef URL) bu tek-makineli test ortaminda DNS/`/etc/hosts`'ta hic
kayitli degildi (port 19052 kendisi dinliyordu, sadece isim cozulmuyordu)
-- `/etc/hosts`'a elle bir satir eklenerek cozuldu. Ikisi de AutoSync'in
degil, ortamin eksikligiydi. Duzeltildikten sonra `fullindex --clear-acl`
(clearacl dahil TUM adimlar: validate->clearacl->clearindex->stopindex->
fullindex_start->aftervalidate) EXIT=0 ile tamamlandi; `swapmon.Cleanup()`
duzeltmesi de burada ayrica teyit edildi (artik hata degil, bilgilendirici
"temizlenecek bir sey yok" satiri basiyor).

**Kalan tek acik madde:** `3dindex`'in tam ucdan uca (gercek crawler
calismasiyla) dogrulanmasi icin, VM snapshot'tan GERCEK `3DIndexes`
icerigiyle geri yuklenmis bir ortam gerekiyor -- bu, veri kazayla
kaybedildigi icin bu oturumlarda yapilamadi. Kod tarafi (env/sarmalayici
cagrisi) zaten dogrulandi, sadece gercek crawler calismasi (BBDAdminPlayer)
test edilemedi.

## TAM `orchestrate` QA dongusu basariyla tamamlandi (2026-11-11/16)

`3dsync-agent-run orchestrate` (yani `run_full_sync.sh`'in TAM Go
karsiligi) BASTAN SONA, GERCEK ortamda, EXIT=0 ile calisti -- projenin
en kapsamli dogrulamasi. Sirasiyla: service-stop -> task-stop -> FCS
kopyalama (arka planda, paralel) -> db-sync (gercek backup+restore,
159377 sayfa) -> store-sync -> config-sync (URL donusumu dogrulandi)
-> clear-logs-and-start-services (TUM TomEE servisleri + CloudView
GERCEKTEN basladi) -> verify-services -> fullindex -> restart-scheduler.

Bu sirada IKI gercek hata daha bulunup duzeltildi (toplam 8):

7. **`orchestrate.go`'da AYNI bug sinifi UCUNCU KEZ** -- `Config.AgentPath`
   de (fonksiyon `FromEnvAndFlags`) ham binary'yi (`3dsync-agent`)
   donduruyordu, sarmalayiciyi degil; orchestrate host0'dan KENDISINE
   SSH ile `agentStep()` cagirdigi icin, bu da env.sh'nin hic
   kaynaklanmamasina yol aciyordu. Ayrica `step.run()` da `fanout1()`
   ile ayni UX/guvenlik riskini tasiyordu (buffer'lanmis cikti) --
   ikisi de duzeltildi (`RunLive` kullaniliyor artik).
8. **Legacy `steps/10_clear_logs_and_start_services.sh`'de gercek
   ortam uyumsuzlugu**: bu test ortaminda HAProxy hic kurulu degildi
   (`command not found`), ve `&&` zinciri bunu hic yakalamadan TUM
   senkronu durduruyordu. `command -v haproxy` kontrolu eklenerek
   LB yazilimi yoksa/farkliysa adim artik sadece bilgilendirici bir
   not basip devam ediyor (Go tarafindaki `verify.LB()` otomatik-tespit
   mantigiyla AYNI ruh). **Bu duzeltmenin ILK halinde** ayrica KENDI
   eklenen Turkce metinde bir kesme-isareti (`host'ta`) bash tek-tirnak
   stringini erken kapatip "unexpected token `('" hatasina yol acti --
   bu da bulunup duzeltildi ("host uzerinde" olarak degistirildi).

**Sonuc:** Butun ana adimlar (service-stop, task-stop, FCS, db-sync,
store-sync, config-sync, servis baslatma, verify-services, fullindex,
restart-scheduler) artik gercek ortamda, TEK bir `orchestrate` komutuyla,
basindan sonuna EXIT=0 ile dogrulanmis durumda. Kalan tek acik nokta,
`3dindex`'in gercek crawler verisiyle (VM snapshot restore sonrasi)
dogrulanmasidir -- bu, veri kaybi nedeniyle bu oturumda yapilamadi,
kod tarafi (env/sarmalayici) zaten dogrulandi.

## Henüz yapılmadı (kapsam dışı bırakıldı, bilerek)

- Lisans/aktivasyon kontrolü (host-fingerprint + süre) — ayrı bir konu,
  istenirse `cmd/3dsync-agent/main.go`'ya bir `licensecheck` paketi olarak
  eklenebilir.
- `secrets.sh`'in her sunucuya düz metin `scp` edilmesi sorunu (bkz. önceki
  güvenlik bulgusu) — bu göçten bağımsız, ayrıca çözülmeli.
- `setup/02_deploy.sh`'a, artık her host'a `.sh` dosyaları yanında
  `3dsync-agent` binary'sinin de deploy edilmesi eklenmeli (bkz. "Deploy"
  bölümü) — bu script henüz migrate edilmedi, elle güncellenmesi gerekiyor.
