#!/bin/bash
# =============================================================================
# 3dsync-agent -- env.sh SABLONU
# =============================================================================
# Bu dosya, her musteri/ortam icin "./3dsync-agent init-config" ile
# doldurulacak REFERANS sablondur (bkz. internal/preflight/preflight.go --
# asagidaki her degisken o dosyadaki requiredVars/optionalVars listesiyle
# BIREBIR eslesir; sira da ORADAKI sirayla aynidir).
#
# KULLANIM:
#   ./3dsync-agent init-config --output customers/<ad>/env.sh
#   (varsayilan --template bu dosyadir: config/env.sh)
#
# Doldurduktan sonra DOGRULAYIN:
#   ./3dsync-agent validate-config customers/<ad>/env.sh --skip-ssh
#   (SSH anahtarlari kurulduktan SONRA, --skip-ssh OLMADAN tekrar)
#
# NOT: Bu sablon GERCEK bir musteri degeri icermez -- sadece yer
# tutucu ve aciklama. secrets.sh (parolalar) BU DOSYADA DEGIL, ayri
# ve git'e girmeyen bir dosyada tutulur (bkz. customers/README.md).
# =============================================================================

# ---------------------------------------------------------------------------
# Host rolleri (SSH ile erisilecek makineler)
# ---------------------------------------------------------------------------
# SSH/isletim sistemi kullanicisi (TUM host'larda AYNI olmali -- orchestrate
# bu kullaniciyla host0'dan diger host'lara sifresiz SSH fan-out yapar)
user=CHANGE_ME_ssh_user

# Ana Makine (Main/Control) -- orchestrate BURADAN calistirilir, diger
# host'lara buradan SSH ile baglanilir
host0=CHANGE_ME_host0

# Hedef Veritabani sunucusu (SQL Server)
host1=CHANGE_ME_host1

# Passport/Comment/Dashboard/Swym/FedSearch sunucusu
host2=CHANGE_ME_host2

# 3DSpace/3DSpaceInternal sunucusu
host3=CHANGE_ME_host3

# 3DSpaceIndex/3DIndex sunucusu
host4=CHANGE_ME_host4

# FCS Central sunucusu
host5=CHANGE_ME_host5

# HAProxy/LB sunucusu -- OPSIYONEL, yoksa bos birakin (araclar otomatik atlar)
host6=CHANGE_ME_host6

# Kaynak FCS sunucusu
host7=CHANGE_ME_host7

# Kaynak Veritabani sunucusu
host8=CHANGE_ME_host8

# FCS Remote sunucusu
host9=CHANGE_ME_host9

# ---------------------------------------------------------------------------
# Yollar
# ---------------------------------------------------------------------------
# 3dsync-agent'in kendisinin dagitildigi/calistigi dizin (TUM host'larda
# AYNI yol olmali -- install.sh binary'yi buraya kopyalar)
mainpath=CHANGE_ME_/opt/3dsync-agent

# 3DEXPERIENCE kurulum kok dizini
exppath=CHANGE_ME_/opt/3DEXPERIENCE

# Hedef DB gecici dizini (host1 uzerinde, backup/restore icin)
sqlpath=CHANGE_ME_/opt/mssql/tmp

# Kaynak DB gecici dizini (host8 uzerinde)
sqlpath2=CHANGE_ME_/opt/mssql/tmp

# SQL Server data dizini (host1)
sqldata=CHANGE_ME_/var/opt/mssql/data

# SQL Server log dizini (host1)
sqllog=CHANGE_ME_/var/opt/mssql/log

# SQL Server index dizini (host1) -- OPSIYONEL, boşsa izin adimi atlanir
sqlindex=CHANGE_ME_/var/opt/mssql/index

# FCS hedef kok dizini (host3)
dest1=CHANGE_ME_/fcs/dest

# FCS hedef ikincil dizin -- OPSIYONEL
dest2=CHANGE_ME_/fcs/dest2

# FCS kaynak kok dizini (host7)
src1=CHANGE_ME_/fcs/src

# FCS kaynak ikincil dizin -- OPSIYONEL
src2=CHANGE_ME_/fcs/src2

# ---------------------------------------------------------------------------
# Veritabani
# ---------------------------------------------------------------------------
# Veritabani adi (orn. "3DSpace")
DB_NAME="CHANGE_ME_spacedb"

# SQL Server kullanici adi (DIKKAT: buyuk 'U' ile -- koddaki degisken adi
# tam olarak "Username")
Username="CHANGE_ME_sa"

# SQL Server 'sa' parolasi -- BU DEGERI BURAYA YAZMAYIN. Bu satiri silip
# gercek parolayi customers/<ad>/secrets.sh icine "password=..." olarak
# koyun (secrets.sh asla git'e girmez, env.sh girer).
password=CHANGE_ME_bu_degeri_secrets_sh_icine_tasiyin

# ---------------------------------------------------------------------------
# URL/config.xml donusumu (internal/configgen)
# ---------------------------------------------------------------------------
# config.xml icinde ARANAN metin (kaynak ortam URL/host adi)
urlconf1=CHANGE_ME_kaynak-url-parcasi

# config.xml icine YAZILACAK metin (hedef ortam URL/host adi)
urlconf2=CHANGE_ME_hedef-url-parcasi

# Ikinci (opsiyonel) donusum ciftin kaynagi -- ikisi de BOSSA atlanir
urlconf3=

# Ikinci (opsiyonel) donusum ciftin hedefi
urlconf4=

# ---------------------------------------------------------------------------
# STORE hedefleri (plmx/store/imagestore)
# ---------------------------------------------------------------------------
# STORE plmx hedef URL
plmxurl=CHANGE_ME_plmx-hedef-url

# STORE store hedef URL
storeurl=CHANGE_ME_store-hedef-url

# STORE imagestore hedef URL
imageurl=CHANGE_ME_imagestore-hedef-url

# plmx/store/imagestore yerel dizinleri (host3 uzerinde) -- genelde
# exppath altindan turetilir, gerekirse ACIKCA override edin
plmxpath=CHANGE_ME_/opt/3DEXPERIENCE/plmx
storepath=CHANGE_ME_/opt/3DEXPERIENCE/store
imagepath=CHANGE_ME_/opt/3DEXPERIENCE/imagestore

# ---------------------------------------------------------------------------
# TomEE servis yollari (host2/host3/host4/host5 -- verify + stop/start icin)
# ---------------------------------------------------------------------------
commenttomeepath=CHANGE_ME_/opt/3DEXPERIENCE/tomee/comment
dashboardtomeepath=CHANGE_ME_/opt/3DEXPERIENCE/tomee/dashboard
passporttomeepath=CHANGE_ME_/opt/3DEXPERIENCE/tomee/passport
fedsearchtomeepath=CHANGE_ME_/opt/3DEXPERIENCE/tomee/fedsearch
swymtomeepath=CHANGE_ME_/opt/3DEXPERIENCE/tomee/swym
spacetomeepath=CHANGE_ME_/opt/3DEXPERIENCE/tomee/3dspace
internaltomeepath=CHANGE_ME_/opt/3DEXPERIENCE/tomee/3dspaceinternal
fcstomeepathcentral=CHANGE_ME_/opt/3DEXPERIENCE/tomee/fcscentral
fcstomeepathremote=CHANGE_ME_/opt/3DEXPERIENCE/tomee/fcsremote

# HAProxy/LB servis adi -- OPSIYONEL (host6 bossa bu da genelde bossa
# birakilir; araclar LB'yi otomatik tespit etmeye calisir)
lbservice=

# ---------------------------------------------------------------------------
# Yardimci / opsiyonel ayarlar (CHANGE_ME_ DEGIL -- calisan varsayilan
# degerler; ihtiyaciniz yoksa dokunmadan birakabilirsiniz)
# ---------------------------------------------------------------------------
# DB yedek dosya adlari (db-backup-run/db-restore-run) -- bossa araclar
# kendi varsayilan adini uretir
BACKUP_FILE=
BACKUP_FILE2=

# orchestrate --clear-acl bayragini komut satirinda vermeden de ayni
# etkiyi ortam degiskeniyle istiyorsaniz "1" yapin (varsayilan: kapali)
CLEAR_ACL=0

# wait-for-hosts icin bekleme ayarlari (saniye) -- varsayilanlar genelde
# yeterlidir
WAIT_FOR_HOSTS_MAX=300
WAIT_FOR_HOSTS_INTERVAL=5

# SQLCMD araci dizini -- bossa PATH'teki sqlcmd kullanilir
sqlcmd=

# swapmon icin taban dizin -- bossa swap kontrolu atlanir
swap_base_dir=

# "Programs" hedef dizini ACIKCA override etmek icin -- bossa varsayilan
# konum kullanilir
enoconfigdir=
