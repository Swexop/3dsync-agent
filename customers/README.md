# customers/ — müşteriye özel kurulum dosyaları

Her müşteri için `customers/<musteri-adi>/` altında üç dosya tutulur.
`install.sh --customer <ad>` ve `rollback.sh --customer <ad>` bu üçünü
otomatik olarak bu konumdan okur:

| Dosya | İçerik | Git'e girer mi? | Nasıl üretilir |
|---|---|---|---|
| `env.sh` | Host listesi, `mainpath`, `urlconf` vb. — **gizli değil**. | Evet (git'e eklenebilir). | `./3dsync-agent init-config --output customers/<ad>/env.sh` |
| `secrets.sh` | DB/servis parolaları — **gizli**. | **Hayır** — `.gitignore`'da `customers/*/secrets.sh` deseniyle dışarıda tutulur. | Elle hazırlanır, `chmod 600`. |
| `license.lic` | Ed25519 imzalı lisans — müşteriye özel, **sır değil ama paylaşılmamalı**. | **Hayır** — `.gitignore`'da `*.lic` deseniyle dışarıda tutulur. | `go run ./cmd/license-gen ...` (SADECE KhanTech tarafında, private key ile). |

`env.sh` için gerçek bir referans şablon bu repoda `config/env.sh`
altında bulunur (tüm değişkenler `CHANGE_ME_` ile işaretli, her birinin
üstünde açıklama var) — `init-config`'in varsayılan `--template`'i da
budur, yani `--template` belirtmeden sadece `--output` vermeniz yeterli:

```bash
./3dsync-agent init-config --output customers/<ad>/env.sh
```

**Önemli:** `validate-config` bir dosya yolu ALMAZ — `env.sh`'i önce
kendi shell'inize source etmeniz, SONRA komutu çalıştırmanız gerekir:

```bash
set -a; . customers/<ad>/env.sh; set +a
./3dsync-agent validate-config --skip-ssh
```

Tam adım adım kurulum rehberi için bkz. proje dokümantasyonundaki
"Yeni Müşteriye Kurulum Rehberi" (ya da bu repoyla birlikte dağıtılan
`README_MIGRATION.md`).

Bu dizin bilerek repoya `customers/README.md` dışında müşteri verisi
içermez — her müşterinin kendi `env.sh`/`secrets.sh`/`license.lic`'i
kendi alt klasöründe, yukarıdaki tabloya göre oluşturulur.
