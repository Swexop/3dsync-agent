# 3dsync-agent — Baştan Sona Kurulum Adımları

Bu liste, paketi aldığınız andan (tar.gz dosyası) ilk `orchestrate`
çalıştırmasına kadar izlenecek TÜM adımları sırayla verir. Her adımı
sırayla tamamlayın, bir öncekini bitirmeden sonrakine geçmeyin.

Detaylı anlatım için: "3dsync-agent — Yeni Müşteriye Kurulum Rehberi"
(Claude Docs) — https://claude.ai/artifact/Aq8f86BfuffLAdMVTmvtFM

Bu dosyada `<musteri-adi>` geçen her yere kendi müşteri/ortam adınızı
(örn. `test`, `musteri1`) yazın.

---

## 0. Paketi sunucuya taşı ve aç

```bash
# Paket zaten sunucudaysa bu adımı atlayın.
scp 3dsync-deploy-package-*.tar.gz kullanici@sunucu:/hedef/dizin/

ssh kullanici@sunucu
cd /hedef/dizin
tar -xzf 3dsync-deploy-package-*.tar.gz
cd 3dsync-deploy-package
chmod +x 3dsync-agent install.sh rollback.sh
```

**Kontrol:** Aşağıdaki dosyalar klasörde olmalı:

```bash
ls -la
# 3dsync-agent  install.sh  rollback.sh  config/env.sh
# customers/README.md  README_MIGRATION.md  PAKET_OKUBENI.txt
```

## 1. Binary'nin çalıştığını doğrula

```bash
./3dsync-agent version
```

`3dsync-agent vX.Y.Z-...` basmalı. Hata veriyorsa (izin, mimari
uyumsuzluğu vb.) diğer adımlara geçmeyin, önce bunu çözün.

## 2. Müşteri dizinini oluştur

```bash
mkdir -p customers/<musteri-adi>
```

## 3. env.sh üret (init-config)

```bash
./3dsync-agent init-config --output customers/<musteri-adi>/env.sh
```

Sırayla sorulan değerleri (user, host0..host9, mainpath, exppath, DB
bilgileri, urlconf1/urlconf2, STORE URL'leri, TomEE yolları — toplam 42
değer) kendi ortamınıza göre girin. Hazır bir cevap listeniz varsa:

```bash
./3dsync-agent init-config --output customers/<musteri-adi>/env.sh \
  --answers customers/<musteri-adi>/env.answers --non-interactive
```

## 4. secrets.sh hazırla (elle)

```bash
touch customers/<musteri-adi>/secrets.sh
chmod 600 customers/<musteri-adi>/secrets.sh
```

Dosyanın içine DB/servis parolalarını girin (örn. `password=...`).
**Bu dosyayı git'e eklemeyin** — `.gitignore` zaten engelliyor ama yine
de `git status` ile kontrol edin.

## 5. env.sh'i doğrula (validate-config)

**Önemli:** `validate-config` bir dosya yolu ALMAZ — önce `env.sh`'i
kendi shell'inize source etmeniz gerekir:

```bash
set -a
. customers/<musteri-adi>/env.sh
set +a
./3dsync-agent validate-config --skip-ssh
```

Tüm değişkenler `[OK]` görünmeli. `[HATA]` varsa `env.sh`'e geri dönüp
düzeltin, sonra bu adımı tekrarlayın.

## 6. Lisans üret (license-gen) — SADECE KhanTech tarafında

```bash
go run ./cmd/license-gen \
  -privkey /guvenli/yol/license-private.key \
  -customer "<Musteri Adi>" \
  -expires 2027-11-16 \
  -out customers/<musteri-adi>/license.lic
```

`cmd/license-gen` bu pakette YOK (bilerek) — bu adım sadece private
key'e sahip olan kişi/makinede çalıştırılır, üretilen `license.lic`
dosyası sonra `customers/<musteri-adi>/` altına kopyalanır.

İsteğe bağlı: belirli bir host'a sabitlemek isterseniz, hedef host'ta
önce fingerprint alın:

```bash
printf '%s' "$(cat /etc/machine-id)" | sha256sum | awk '{print $1}'
```

ve bu değeri `-host-fingerprint <hex>` olarak license-gen'e verin
(ilk kurulumda genelde ATLANIR — yanlış host'a sabitlemek kurulumu
kilitleyebilir).

## 7. install.sh --dry-run ile önizle

```bash
./install.sh --customer <musteri-adi> --dry-run
```

Hiçbir host'a dokunulmaz. Çıktıda host listesini, hangi dosyaların
kopyalanacağını ve SSH key kurulum adımını kontrol edin. Bir şey
yanlışsa `env.sh`'e dönüp düzeltin, bu adımı tekrarlayın.

## 8. Gerçek kurulum (install.sh)

```bash
./install.sh --customer <musteri-adi>
```

Binary'yi (gerekirse derler), `3dsync-agent-run` sarmalayıcısını,
`env.sh`/`secrets.sh`/`license.lic`'i tüm host'lara kopyalar ve host0'dan
diğer host'lara şifresiz SSH key kurar. **Hiçbir `-run`/`trigger` komutu
ÇALIŞTIRMAZ** — sadece yerleştirir.

## 9. SSH key kurulumunu manuel doğrula

```bash
ssh <user>@host0
# host0 icindeyken, env.sh'deki HER host icin:
ssh <user>@host1 'echo OK'
ssh <user>@host2 'echo OK'
# ...
```

Parola sorulmadan `OK` dönmeli. Sormuyorsa `install.sh`'i
`--no-ssh-keys` OLMADAN tekrar çalıştırın veya key'i elle ekleyin.

## 10. Salt-okunur komutlarla test

```bash
ssh $user@<host> "$mainpath/3dsync-agent-run version"
ssh $user@<host> "$mainpath/3dsync-agent-run verify-services"
```

`version` lisanssız da çalışır. `verify-services` lisans ister —
başarısız olursa `license.lic`'in o host'ta `$mainpath/config/`
altında ve geçerli olduğunu kontrol edin.

## 11. İlk orchestrate çalıştırması

```bash
ssh $user@<orchestrator-host> "$mainpath/3dsync-agent-run orchestrate"
```

Bitince otomatik sağlık raporu yazılır:

```bash
ssh $user@<orchestrator-host> "cat $mainpath/SyncResult/health_report_*.txt | tail -1"
```

`SONUC: BASARILI` görmelisiniz. Mümkünse düşük trafikli bir saatte ve
bir yedek penceresi varken çalıştırın.

## 12. Sorun çıkarsa: rollback

```bash
./rollback.sh --customer <musteri-adi> --list       # mevcut yedekleri listele
./rollback.sh --customer <musteri-adi> --dry-run     # ne geri yuklenecek, SADECE goster
./rollback.sh --customer <musteri-adi>               # EN SON yedegi geri yukle
./rollback.sh --customer <musteri-adi> --to <TS>     # BELIRLI bir yedegi geri yukle
```

---

## Kapanış kontrol listesi

- [ ] Paket açıldı, `3dsync-agent version` çalıştı
- [ ] `customers/<ad>/` oluşturuldu
- [ ] `env.sh` üretildi (`init-config`)
- [ ] `secrets.sh` elle hazırlandı, `chmod 600`, git'e eklenmedi
- [ ] `env.sh` source edilip `validate-config --skip-ssh` ile doğrulandı — tüm `[OK]`
- [ ] `license.lic` üretildi (KhanTech tarafında, `license-gen` ile)
- [ ] `install.sh --dry-run` çıktısı gözden geçirildi
- [ ] `install.sh` ile gerçek kurulum yapıldı
- [ ] SSH key kurulumu manuel doğrulandı (host0 → diğer tüm host'lar, parolasız)
- [ ] `version` ve `verify-services` salt-okunur testleri başarılı
- [ ] İlk `orchestrate` çalıştırıldı, sağlık raporu `SONUC: BASARILI` gösterdi
- [ ] Rollback komutları ve yedek konumu not edildi
