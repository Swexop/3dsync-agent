#!/usr/bin/env bash
# install.sh -- 3dsync-agent'i derler ve config/env.sh'de tanimli
# host0..host9'un HEPSINE binary + bir cagri sarmalayicisi (3dsync-agent-run)
# dagitir. Tek komutla kurulum icin yazildi.
#
# KULLANIM:
#   ./install.sh /path/to/config/env.sh [/path/to/config/secrets.sh] [/path/to/config/license.lic]
#   ./install.sh --customer <ad>
#     -- yukaridakinin kisayoli: customers/<ad>/env.sh,
#     customers/<ad>/secrets.sh VE customers/<ad>/license.lic
#     dosyalarini kullanir (COK SAYIDA musteri/kurulumu ayri
#     dizinlerde tutmak icin -- bkz. customers/README.md).
#   license.lic OPSIYONELDIR ama ONEMLIDIR: 3dsync-agent, "version"/
#   "validate-config"/"init-config" DISINDAKI HICBIR komutu (orchestrate
#   dahil) gecerli bir license.lic OLMADAN CALISTIRMAZ (bkz.
#   internal/license). Bu dosya license-gen araciyla (KhanTech ic
#   kullanimi, musteriye gitmez) uretilir.
#   --dry-run    HICBIR host'a SSH/scp ATMADAN (ve SSH key kurmadan),
#                sadece NELERIN yapilacagini listeler. Yeni bir musteriye
#                ilk kurulumdan once, veya env.sh degisikliginden sonra
#                "bu komut tam olarak neyi degistirecek?" sorusuna cevap
#                vermek icin -- bkz. Faz 4 (urunlestirme yol haritasi).
#
# NOT: env.sh ve secrets.sh BU PAKETTE YOK -- musterinin kendi AutoSync/
# 3DSync kurulumundan (mevcut, degismeyen dosyalar) geliyor. Bu script
# onlari SADECE host listesini (host0..host9) ve user/mainpath
# degerlerini OKUMAK icin source eder; hicbir degeri ekrana yazdirmaz,
# degistirmez, baska bir yere kopyalamaz.
#
# NE YAPAR:
#   1. Go kuruluysa binary'yi derler (linux/amd64, stripped); degilse
#      bu dizindeki onceden derlenmis './3dsync-agent' dosyasini kullanir.
#   2. Kucuk bir "3dsync-agent-run" sarmalayici uretir -- bu, her host'ta
#      config/env.sh (+ secrets.sh varsa) otomatik kaynaklayip binary'ye
#      iletir; siz elle "source env.sh && ..." yazmazsiniz.
#   3. env.sh'den host0..host9'u toplar (tekrarlari eler), her birine
#      scp ile binary + sarmalayiciyi KOPYALAR, chmod +x yapar; AYRICA
#      config/env.sh (+secrets.sh, +license.lic -- hangileri verildiyse)
#      dosyalarini da HER host'a ($mainpath/config/ altina) kopyalar --
#      sarmalayici, ic SSH fan-out'u sirasinda her hostun KENDI env.sh'ini
#      kaynaklamak zorunda (nested SSH, surecin ortam degiskenlerini
#      miras almaz); lisans kontrolu de HER host'un KENDI license.lic'ine
#      bakar (bkz. internal/license), bu yuzden her host'a kopyalanmasi
#      gerekir.
#   4. host0'da (yoksa) bir SSH anahtari olusturur ve bunu DIGER TUM
#      host'larin ~/.ssh/authorized_keys'ine ekler -- "trigger" komutlari
#      (verify-services, service-stop, store-sync, vb.) host0'dan diger
#      host'lara SIFRESIZ SSH ile fan-out yapmak ZORUNDA (script icinden
#      tetiklendikleri icin parola isteyemezler) -- bu adim olmadan HEPSI
#      "Permission denied" ile duser. --no-ssh-keys ile atlanabilir.
#
# NE YAPMAZ: hicbir "-run"/"trigger" komutunu CALISTIRMAZ -- sadece
# dosyalari yerlestirir ve SSH key kurar. Ilk calistirma denemesi icin
# README_MIGRATION.md icindeki "once sadece-okuyan komutlarla test edin"
# bolumune bakin.

set -euo pipefail

SETUP_SSH_KEYS=1
DRY_RUN=0
CUSTOMER=""
POSITIONAL=()
while [ $# -gt 0 ]; do
  case "$1" in
    --no-ssh-keys) SETUP_SSH_KEYS=0; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    --customer)
      CUSTOMER="${2:-}"
      if [ -z "$CUSTOMER" ]; then
        echo "HATA: --customer icin bir ad verilmedi." >&2
        exit 1
      fi
      shift 2
      ;;
    *) POSITIONAL+=("$1"); shift ;;
  esac
done

SCRIPT_DIR_EARLY="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -n "$CUSTOMER" ]; then
  if [ "${#POSITIONAL[@]}" -gt 0 ]; then
    echo "HATA: --customer, elle verilen env.sh/secrets.sh yollariyla BIRLIKTE kullanilamaz." >&2
    exit 1
  fi
  ENV_FILE="$SCRIPT_DIR_EARLY/customers/$CUSTOMER/env.sh"
  SECRETS_FILE="$SCRIPT_DIR_EARLY/customers/$CUSTOMER/secrets.sh"
  LICENSE_FILE="$SCRIPT_DIR_EARLY/customers/$CUSTOMER/license.lic"
  if [ ! -f "$ENV_FILE" ]; then
    echo "HATA: '$ENV_FILE' bulunamadi -- once bu musteri icin env.sh'i olusturun, orn.:" >&2
    echo "        ./3dsync-agent init-config --template AutoSync/config/env.sh \\" >&2
    echo "            --answers customers/$CUSTOMER/env.answers --output customers/$CUSTOMER/env.sh --non-interactive" >&2
    echo "      (bkz. customers/README.md)" >&2
    exit 1
  fi
else
  ENV_FILE="${POSITIONAL[0]:-}"
  SECRETS_FILE="${POSITIONAL[1]:-}"
  LICENSE_FILE="${POSITIONAL[2]:-}"
fi

if [ -z "$ENV_FILE" ] || [ ! -f "$ENV_FILE" ]; then
  echo "Kullanim: $0 /path/to/config/env.sh [/path/to/config/secrets.sh] [/path/to/config/license.lic] [--no-ssh-keys] [--dry-run]" >&2
  echo "      veya: $0 --customer <ad> [--no-ssh-keys] [--dry-run]" >&2
  exit 1
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "==> --dry-run VERILDI: hicbir host'a SSH/scp atilmayacak, hicbir dosya"
  echo "    degistirilmeyecek/kopyalanmayacak, hicbir SSH anahtari kurulmayacak."
  echo "    Sadece NELERIN yapilacagi listelenecek."
  echo
fi

# shellcheck disable=SC1090
. "$ENV_FILE"
if [ -n "$SECRETS_FILE" ] && [ -f "$SECRETS_FILE" ]; then
  # shellcheck disable=SC1090
  . "$SECRETS_FILE"
fi

: "${user:?user tanimli degil -- env.sh dogru kaynaklandi mi?}"
: "${mainpath:?mainpath tanimli degil -- env.sh dogru kaynaklandi mi?}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$SCRIPT_DIR/3dsync-agent"

# --- 1) Binary'yi derle (go varsa) -------------------------------------
# NOT (--dry-run): bu adim DRY_RUN'da da GERCEKTEN calisir -- hicbir
# host'a dokunmuyor (sadece bu dizindeki yerel binary'yi uretiyor/
# guncelliyor), ve sonraki adimlarin ("nelerin gonderilecegi") dogru
# bir binary uzerinden raporlanmasini saglar.
if command -v go >/dev/null 2>&1; then
  echo "==> go build (linux/amd64, stripped)..."
  # Surum numarasini git tag'inden oku (yoksa "dev") -- bkz. build.sh.
  if git -C "$SCRIPT_DIR" rev-parse --git-dir >/dev/null 2>&1; then
    BUILD_VERSION="$(git -C "$SCRIPT_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)"
  else
    BUILD_VERSION="dev"
  fi
  ( cd "$SCRIPT_DIR" && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.buildVersion=$BUILD_VERSION" -o 3dsync-agent ./cmd/3dsync-agent )
elif [ -x "$BINARY" ]; then
  echo "==> 'go' bulunamadi, mevcut '$BINARY' kullanilacak (yeniden derlenmedi)."
else
  echo "HATA: 'go' bulunamadi ve onceden derlenmis '$BINARY' da yok." >&2
  echo "      Once su komutu calistirin:" >&2
  echo "        GOOS=linux GOARCH=amd64 go build -ldflags=\"-s -w\" -o 3dsync-agent ./cmd/3dsync-agent" >&2
  exit 1
fi

# --- 1.5) validate-config: dagitimdan ONCE env.sh'i dogrula ------------
# AMAC (productization): "CHANGE_ME_..." sablon degerlerinden biri
# unutulmus mu, host'lara SSH ile erisiliyor mu -- bunlari dagitimdan
# SONRA, orta bir senkron adiminda anlasilmasi zor bir hatayla degil,
# dagitimdan ONCE, tek bir komutla yakalamak icin. --skip-validate ile
# atlanabilir (orn. hedef host'lara henuz SSH key kurulmadiysa, zincirin
# "once binary'yi koy, sonra anahtar kur" sirasinda).
if [ "${SKIP_VALIDATE:-0}" != "1" ]; then
  echo "==> validate-config calistiriliyor (env.sh dogrulaniyor, henuz bir sey degistirilmiyor)..."
  if ! "$BINARY" validate-config --skip-ssh; then
    echo "HATA: validate-config basarisiz oldu -- yukaridaki [HATA] satirlarini env.sh'de duzeltin." >&2
    echo "      (SSH anahtarlari henuz kurulmadiysa, sadece bu kontrolu atlamak icin:" >&2
    echo "       SKIP_VALIDATE=1 $0 $* )" >&2
    exit 1
  fi
  echo
fi

# --- 2) Cagri sarmalayicisini uret -------------------------------------
WRAPPER="$(mktemp)"
trap 'rm -f "$WRAPPER"' EXIT
cat > "$WRAPPER" <<'EOF'
#!/bin/sh
# 3dsync-agent-run -- config/env.sh (+ secrets.sh varsa, bu host'un
# KENDI dizininde) otomatik kaynaklanip 3dsync-agent'e iletilir.
# install.sh tarafindan uretildi, elle duzenlemeyin.
DIR="$(cd "$(dirname "$0")" && pwd)"
. "$DIR/config/env.sh"
if [ -f "$DIR/config/secrets.sh" ]; then
  . "$DIR/config/secrets.sh"
fi
exec "$DIR/3dsync-agent" "$@"
EOF
chmod +x "$WRAPPER"

# --- 3) Host listesini topla (host0..host9, tekrarsiz) -----------------
HOSTS=""
for i in $(seq 0 9); do
  var="host$i"
  val="${!var:-}"
  if [ -n "$val" ]; then
    case " $HOSTS " in
      *" $val "*) ;;
      *) HOSTS="$HOSTS $val" ;;
    esac
  fi
done

if [ -z "$HOSTS" ]; then
  echo "HATA: env.sh'de host0..host9 icin hicbir deger bulunamadi." >&2
  exit 1
fi

echo "==> Dagitilacak host'lar:$HOSTS"

HAS_SECRETS=0
if [ -n "$SECRETS_FILE" ] && [ -f "$SECRETS_FILE" ]; then
  HAS_SECRETS=1
fi

HAS_LICENSE=0
if [ -n "${LICENSE_FILE:-}" ] && [ -f "$LICENSE_FILE" ]; then
  HAS_LICENSE=1
fi
if [ "$HAS_LICENSE" -eq 0 ]; then
  echo "UYARI: license.lic verilmedi/bulunamadi -- 3dsync-agent bu host'larda"
  echo "       'version'/'validate-config'/'init-config' DISINDAKI HICBIR komutu"
  echo "       (orchestrate dahil) CALISTIRMAYACAK. Lisans dosyasi daha sonra"
  echo "       elle '\$mainpath/config/license.lic' olarak her host'a kopyalanabilir."
  echo
fi

# INSTALL_TS: bu calistirmaya ait TEK bir zaman damgasi -- asagidaki
# yedekleme adiminda HER host'ta AYNI klasor adi (.rollback/$INSTALL_TS/)
# kullanilir, boylece bir sonraki "rollback.sh" cagrisi butun host'lari
# TUTARLI (ayni ana dagitimdan ONCEKI) bir noktaya geri alabilir --
# bkz. rollback.sh.
INSTALL_TS="$(date +%Y%m%d_%H%M%S)"

FAILED=""
for h in $HOSTS; do
  echo "--- $h ---"
  # NOT: config/env.sh (+secrets.sh varsa) de HER host'a kopyalaniyor --
  # "3dsync-agent-run" sarmalayicisi, trigger komutlarinin ic SSH
  # fan-out'u sirasinda KENDI hostundaki config/env.sh'i kaynaklamak
  # ZORUNDA (binary, baska bir hosta SSH attiginda bu surecin ortam
  # degiskenlerini miras ALMAZ) -- yani her host kendi env.sh/secrets.sh
  # kopyasina sahip olmali. Zaten ayni mainpath'te varsa UZERINE YAZILIR
  # (ayni dosyalar, farkli degil).
  if [ "$DRY_RUN" -eq 1 ]; then
    echo "    [DRY RUN] mkdir -p '$mainpath/.rollback/$INSTALL_TS' + mevcut binary/wrapper/env.sh/secrets.sh'i oraya yedekle (varsa)"
    echo "    [DRY RUN] mkdir -p '$mainpath/config'"
    echo "    [DRY RUN] scp $BINARY -> $user@$h:$mainpath/3dsync-agent"
    echo "    [DRY RUN] scp $WRAPPER -> $user@$h:$mainpath/3dsync-agent-run"
    echo "    [DRY RUN] scp $ENV_FILE -> $user@$h:$mainpath/config/env.sh"
    if [ "$HAS_SECRETS" -eq 1 ]; then
      echo "    [DRY RUN] scp $SECRETS_FILE -> $user@$h:$mainpath/config/secrets.sh"
    fi
    if [ "$HAS_LICENSE" -eq 1 ]; then
      echo "    [DRY RUN] scp $LICENSE_FILE -> $user@$h:$mainpath/config/license.lic"
    fi
    echo "    [DRY RUN] chmod +x binary+wrapper, chmod 600 secrets.sh (varsa)"
    continue
  fi
  # Yedekleme (rollback icin): YENI dosyalari UZERINE YAZMADAN ONCE,
  # bu host'ta HALIHAZIRDA var olan binary/wrapper/env.sh/secrets.sh
  # (varsa -- ilk kurulumda hicbiri yoktur, o zaman bu adim sessizce
  # hicbir sey yapmaz) '.rollback/$INSTALL_TS/' altina kopyalanir.
  # "cp -a" (scp/ssh DEGIL) kullanilir -- zaten AYNI host uzerinde,
  # ag'a gerek yok.
  BACKUP_CMD="mkdir -p '$mainpath/.rollback/$INSTALL_TS' \
    && { [ -f '$mainpath/3dsync-agent' ] && cp -a '$mainpath/3dsync-agent' '$mainpath/.rollback/$INSTALL_TS/' || true; } \
    && { [ -f '$mainpath/3dsync-agent-run' ] && cp -a '$mainpath/3dsync-agent-run' '$mainpath/.rollback/$INSTALL_TS/' || true; } \
    && { [ -f '$mainpath/config/env.sh' ] && cp -a '$mainpath/config/env.sh' '$mainpath/.rollback/$INSTALL_TS/' || true; } \
    && { [ -f '$mainpath/config/secrets.sh' ] && cp -a '$mainpath/config/secrets.sh' '$mainpath/.rollback/$INSTALL_TS/' || true; } \
    && { [ -f '$mainpath/config/license.lic' ] && cp -a '$mainpath/config/license.lic' '$mainpath/.rollback/$INSTALL_TS/' || true; }"
  if ssh -o ConnectTimeout=10 "$user@$h" "$BACKUP_CMD" \
     && ssh "$user@$h" "mkdir -p '$mainpath/config'" \
     && scp -q "$BINARY" "$user@$h:$mainpath/3dsync-agent" \
     && scp -q "$WRAPPER" "$user@$h:$mainpath/3dsync-agent-run" \
     && scp -q "$ENV_FILE" "$user@$h:$mainpath/config/env.sh" \
     && { [ "$HAS_SECRETS" -eq 0 ] || scp -q "$SECRETS_FILE" "$user@$h:$mainpath/config/secrets.sh"; } \
     && { [ "$HAS_LICENSE" -eq 0 ] || scp -q "$LICENSE_FILE" "$user@$h:$mainpath/config/license.lic"; } \
     && ssh "$user@$h" "chmod +x '$mainpath/3dsync-agent' '$mainpath/3dsync-agent-run'; [ -f '$mainpath/config/secrets.sh' ] && chmod 600 '$mainpath/config/secrets.sh'; true"
  then
    echo "    OK (onceki surum varsa .rollback/$INSTALL_TS/ altina yedeklendi)"
  else
    echo "    HATA: $h'a dagitim basarisiz oldu." >&2
    FAILED="$FAILED $h"
  fi
done

if [ "$DRY_RUN" -eq 1 ]; then
  echo
  echo "==> [DRY RUN] Dagitim adimi atlandi (yukarida listelenen islemler GERCEKTE yapilmadi)."
fi

if [ -n "$FAILED" ]; then
  echo
  echo "UYARI: su host'lara dagitim basarisiz oldu:$FAILED" >&2
  exit 1
fi

# --- 4) SSH key kurulumu: host0 -> diger tum host'lar sifresiz --------
#
# "Trigger" komutlari (verify-services, service-stop, store-sync, vb.)
# host0'da calisir ve KENDI ICINDEN diger host'lara SSH ile fan-out
# yapar -- bu cagrilar bir script icinden tetiklendigi icin terminal/TTY
# yok, yani parola SORAMAZLAR. Bu adim olmadan hepsi "Permission denied"
# ile duser (gercek bir ortamda tam boyle oldu). Bash'teki eski
# remote_batch/remote_queue mantigi da ayni sekilde sifresiz SSH'e
# BAGIMLIYDI -- bu yeni bir gereksinim degil, sadece ilk defa otomatik
# tetiklenince fark ediliyor.
if [ "$DRY_RUN" -eq 1 ]; then
  echo
  if [ "$SETUP_SSH_KEYS" -eq 1 ]; then
    if [ -z "${host0:-}" ]; then
      echo "==> [DRY RUN] SSH key kurulumu ATLANIRDI ('host0' env.sh'de tanimli degil)."
    else
      echo "==> [DRY RUN] SSH key kurulumu yapilacakti:"
      echo "      1. $host0'da (yoksa) 'ssh-keygen -t rsa -b 4096' ile anahtar olusturulacak"
      for h in $HOSTS; do
        echo "      2. $host0 -> $h: public key ~/.ssh/authorized_keys'e eklenecek (sifresiz erisim)"
      done
      echo "      3. $host0'dan tum host'lara sifresiz erisim DOGRULANACAK"
    fi
  else
    echo "==> [DRY RUN] --no-ssh-keys verildi, SSH key kurulumu zaten atlanacakti."
  fi
elif [ "$SETUP_SSH_KEYS" -eq 1 ]; then
  if [ -z "${host0:-}" ]; then
    echo
    echo "UYARI: 'host0' env.sh'de tanimli degil -- SSH key kurulumu atlaniyor." >&2
  else
    echo
    echo "==> SSH key kurulumu: $host0 icin anahtar olusturuluyor (yoksa)..."
    PUBKEY_LOCAL="$(mktemp)"

    if ssh -o ConnectTimeout=10 "$user@$host0" \
       'test -f ~/.ssh/id_rsa.pub || ssh-keygen -t rsa -b 4096 -N "" -f ~/.ssh/id_rsa >/dev/null 2>&1; cat ~/.ssh/id_rsa.pub' \
       > "$PUBKEY_LOCAL" && [ -s "$PUBKEY_LOCAL" ]
    then
      KEY_FAILED=""
      for h in $HOSTS; do
        # DIKKAT: host0 KENDISI de dahil -- eger env.sh'de host2/host3
        # gibi bir rol host0 ile AYNI IP'yi kullaniyorsa (kucuk/test
        # ortamlarinda yaygin), trigger komutlari host0'dan host0'a
        # (self-ssh) baglanmaya calisir ve bu da key-based auth
        # gerektirir -- loopback oldugu icin otomatik guvenilmez.
        echo "--- $host0 -> $h (sifresiz erisim ekleniyor) ---"
        REMOTE_TMP="/tmp/.3dsync_pubkey_$$"
        if scp -q "$PUBKEY_LOCAL" "$user@$h:$REMOTE_TMP" \
           && ssh "$user@$h" "mkdir -p ~/.ssh && chmod 700 ~/.ssh && touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && (grep -qxFf '$REMOTE_TMP' ~/.ssh/authorized_keys || cat '$REMOTE_TMP' >> ~/.ssh/authorized_keys); rm -f '$REMOTE_TMP'"
        then
          echo "    OK"
        else
          echo "    UYARI: $h icin authorized_keys guncellenemedi (elle 'ssh-copy-id' deneyin)." >&2
          KEY_FAILED="$KEY_FAILED $h"
        fi
      done

      echo "==> Dogrulama: $host0'dan sifresiz erisim test ediliyor..."
      ssh "$user@$host0" "
        for h in $HOSTS; do
          if ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=accept-new \"\$h\" true 2>/dev/null; then
            echo \"    \$h: sifresiz OK\"
          else
            echo \"    \$h: HALA SIFRESIZ DEGIL (parola veya bilinmeyen host-key sorunu olabilir)\"
          fi
        done
      " || true

      if [ -n "$KEY_FAILED" ]; then
        echo "UYARI: SSH key su host'lara eklenemedi:$KEY_FAILED -- bunlar icin elle 'ssh-copy-id' calistirin." >&2
      fi
    else
      echo "UYARI: $host0'dan public key alinamadi -- SSH key kurulumu atlandi. Elle 'ssh-keygen' + 'ssh-copy-id' yapmaniz gerekebilir." >&2
    fi
    rm -f "$PUBKEY_LOCAL"
  fi
else
  echo
  echo "==> --no-ssh-keys verildi, SSH key kurulumu atlandi."
fi

if [ "$DRY_RUN" -eq 1 ]; then
  cat <<MSG

==> [DRY RUN] tamamlandi -- HICBIR host'a dokunulmadi, hicbir dosya
    kopyalanmadi/degistirilmedi, hicbir SSH anahtari kurulmadi.
    Yukaridaki listeyi gozden gecirin; sorun yoksa ayni komutu
    --dry-run OLMADAN tekrar calistirin.
MSG
else
  cat <<MSG

==> Dagitim tamamlandi.

Simdi her host'ta SADECE-OKUYAN (hicbir seyi degistirmeyen) komutlarla
test edin -- hicbirini canlida gormedik, once bunlarla dogrulayin:

  ssh $user@<host> "$mainpath/3dsync-agent-run version"
  ssh $user@<host> "$mainpath/3dsync-agent-run wait-for-hosts <baska-host>"
  ssh $user@<host> "$mainpath/3dsync-agent-run verify-services"

Bunlar calistiktan sonra README_MIGRATION.md'deki siraya gore (once
izole, geri alinabilir adimlar; fullindex/db-restore'dan once yedek)
devam edin. "orchestrate" komutunu run_full_sync.sh'nin YERINE
gecirmeden once mutlaka bir TEST/QA ortaminda tam calistirin.

NOT: bu dagitimdan ONCEKI binary/wrapper/env.sh/secrets.sh (varsa) her
host'ta '$mainpath/.rollback/$INSTALL_TS/' altina yedeklendi. Bu surum
sorunlu cikarsa: ./rollback.sh $ENV_FILE${SECRETS_FILE:+ $SECRETS_FILE}
MSG
fi
