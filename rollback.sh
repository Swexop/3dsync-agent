#!/usr/bin/env bash
# rollback.sh -- install.sh'in her kurulumdan/dagitimdan ONCE her host'ta
# aldigi yedegi ('.rollback/<zaman-damgasi>/' altina) geri yukler.
#
# NEDEN install.sh'in alt-komutu DEGIL de AYRI, STANDALONE bir script:
# rollback, TANIM GEREGI, "yeni dagitilan binary/config BOZUK/calismiyor
# olabilir" senaryosu icin var. Eger rollback mantigi derlenmis
# 3dsync-agent binary'sinin kendi icinde olsaydı, TAM da o binary'nin
# BOZUK oldugu durumda rollback'in kendisi de calismayabilirdi
# (kacirilmasi gereken bir "tavuk-yumurta" sorunu). Bu yuzden rollback
# SADECE ssh/scp/cp gibi, hedef host'taki 3dsync-agent'in durumundan
# TAMAMEN BAGIMSIZ, duz metin bash komutlariyla calisir -- install.sh'in
# kendisi gibi.
#
# KULLANIM:
#   ./rollback.sh /path/to/config/env.sh [/path/to/config/secrets.sh] [--list] [--to TS] [--dry-run]
#   ./rollback.sh --customer <ad> [--list] [--to TS] [--dry-run]
#
#   --list       Her host'ta mevcut olan yedekleri (zaman damgalarini) listeler,
#                HICBIR SEYI GERI YUKLEMEZ.
#   --to TS      Belirtilen zaman damgasindaki yedegi geri yukler
#                (TS, --list ciktisindaki klasor adlarindan biri, orn.
#                "20261116_143000"). Verilmezse EN SON yedek kullanilir.
#   --dry-run    Hicbir dosyayi degistirmeden, NELERIN geri yuklenecegini listeler.
#
# NE YAPAR (--list/--dry-run DISINDA): her host icin, secilen zaman
# damgasindaki '.rollback/<TS>/' klasorundeki binary/wrapper/env.sh/
# secrets.sh dosyalarini (hangileri o yedekte VARSA) canli konumlarina
# GERI KOPYALAR. Bu islem kendisi de rollback.sh'in SONRAKI bir
# cagrisiyla geri alinabilir degildir -- yani "geri alinani geri almak"
# icin --list ile BASKA bir (daha eski) zaman damgasi secmeniz gerekir.

set -euo pipefail

LIST_ONLY=0
DRY_RUN=0
TARGET_TS=""
CUSTOMER=""
POSITIONAL=()
while [ $# -gt 0 ]; do
  case "$1" in
    --list) LIST_ONLY=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    --to)
      TARGET_TS="${2:-}"
      if [ -z "$TARGET_TS" ]; then
        echo "HATA: --to icin bir zaman damgasi verilmedi." >&2
        exit 1
      fi
      shift 2
      ;;
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
else
  ENV_FILE="${POSITIONAL[0]:-}"
fi

if [ -z "$ENV_FILE" ] || [ ! -f "$ENV_FILE" ]; then
  echo "Kullanim: $0 /path/to/config/env.sh [--list] [--to TS] [--dry-run]" >&2
  echo "      veya: $0 --customer <ad> [--list] [--to TS] [--dry-run]" >&2
  exit 1
fi

# shellcheck disable=SC1090
. "$ENV_FILE"

: "${user:?user tanimli degil -- env.sh dogru kaynaklandi mi?}"
: "${mainpath:?mainpath tanimli degil -- env.sh dogru kaynaklandi mi?}"

# --- Host listesini topla (install.sh ile AYNI mantik) -----------------
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

# --- --list: her host'taki mevcut yedekleri goster, HICBIR SEY DEGISTIRME ---
if [ "$LIST_ONLY" -eq 1 ]; then
  echo "==> Host basina mevcut yedekler ($mainpath/.rollback/):"
  for h in $HOSTS; do
    echo "--- $h ---"
    if ! ssh -o ConnectTimeout=10 "$user@$h" \
      "[ -d '$mainpath/.rollback' ] && ls -1 '$mainpath/.rollback' | sort || echo '(henuz yedek yok)'"
    then
      echo "    UYARI: $h'a baglanilamadi." >&2
    fi
  done
  exit 0
fi

echo "==> Geri yuklenecek host'lar:$HOSTS"
if [ -n "$TARGET_TS" ]; then
  echo "==> Hedef zaman damgasi: $TARGET_TS (elle belirtildi)"
else
  echo "==> Hedef zaman damgasi: EN SON yedek (her host kendi en son yedegini kullanacak)"
fi
if [ "$DRY_RUN" -eq 1 ]; then
  echo "==> --dry-run VERILDI: hicbir dosya degistirilmeyecek, sadece ne yapilacagi gosterilecek."
fi
echo

FAILED=""
for h in $HOSTS; do
  echo "--- $h ---"

  # Bu host icin hangi zaman damgasi kullanilacak: elle verildiyse ONU,
  # yoksa '.rollback/' altindaki EN SON (isim siralamasina gore en
  # buyuk -- zaman damgalari "YYYYMMDD_HHMMSS" formatinda oldugu icin
  # bu ayni zamanda KRONOLOJIK siralamadir) klasoru sec.
  if [ -n "$TARGET_TS" ]; then
    HOST_TS="$TARGET_TS"
  else
    HOST_TS="$(ssh -o ConnectTimeout=10 "$user@$h" \
      "[ -d '$mainpath/.rollback' ] && ls -1 '$mainpath/.rollback' | sort | tail -n1" || true)"
  fi

  if [ -z "$HOST_TS" ]; then
    echo "    UYARI: $h icin kullanilabilir bir yedek bulunamadi, atlaniyor." >&2
    FAILED="$FAILED $h"
    continue
  fi

  BACKUP_DIR="$mainpath/.rollback/$HOST_TS"
  if ! ssh -o ConnectTimeout=10 "$user@$h" "[ -d '$BACKUP_DIR' ]"; then
    echo "    HATA: $h'ta '$BACKUP_DIR' bulunamadi." >&2
    FAILED="$FAILED $h"
    continue
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    echo "    [DRY RUN] $BACKUP_DIR -> $mainpath/{3dsync-agent,3dsync-agent-run,config/env.sh,config/secrets.sh} (hangileri yedekte varsa)"
    continue
  fi

  RESTORE_CMD="cd '$BACKUP_DIR' \
    && { [ -f 3dsync-agent ] && cp -a 3dsync-agent '$mainpath/3dsync-agent' || true; } \
    && { [ -f 3dsync-agent-run ] && cp -a 3dsync-agent-run '$mainpath/3dsync-agent-run' || true; } \
    && { [ -f env.sh ] && cp -a env.sh '$mainpath/config/env.sh' || true; } \
    && { [ -f secrets.sh ] && cp -a secrets.sh '$mainpath/config/secrets.sh' && chmod 600 '$mainpath/config/secrets.sh' || true; } \
    && chmod +x '$mainpath/3dsync-agent' '$mainpath/3dsync-agent-run' 2>/dev/null; true"

  if ssh "$user@$h" "$RESTORE_CMD"; then
    echo "    OK (geri yuklenen zaman damgasi: $HOST_TS)"
  else
    echo "    HATA: $h'ta geri yukleme basarisiz oldu." >&2
    FAILED="$FAILED $h"
  fi
done

if [ -n "$FAILED" ]; then
  echo
  echo "UYARI: su host'larda rollback basarisiz/atlandi:$FAILED" >&2
  exit 1
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo
  echo "==> [DRY RUN] tamamlandi -- hicbir dosya degistirilmedi."
else
  cat <<MSG

==> Rollback tamamlandi.

Simdi her host'ta surum/durumu dogrulayin:
  ssh $user@<host> "$mainpath/3dsync-agent-run version"
  ssh $user@<host> "$mainpath/3dsync-agent-run verify-services"
MSG
fi
