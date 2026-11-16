// Package preflight, yeni bir musteri/kuruluma config/env.sh dogrudan
// elle doldurularak dagitildiginde en sik rastlanan iki hata sinifini
// ERKEN yakalamak icin yazildi:
//
//  1. env.sh sablonundaki "CHANGE_ME_..." yer tutucularindan biri
//     UNUTULUP gercek bir degerle degistirilmemis -- bu durumda
//     senkron, anlasilmasi zor bir hata (orn. "ssh: Could not resolve
//     hostname CHANGE_ME_host3") ile orta bir adimda patlar.
//  2. Bir host alaninda deger VAR ama o host'a SSH ile (key-based,
//     parola sorulmadan) ERISILEMIYOR -- bu da install.sh/orchestrate
//     calisirken "Permission denied"/"Connection timed out" ile
//     ortaya cikar.
//
// "3dsync-agent validate-config" bu ikisini, hicbir senkron adimini
// CALISTIRMADAN, saniyeler icinde raporlar.
package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Result, tek bir kontrolun sonucudur.
type Result struct {
	Name   string // degisken adi (orn. "host3") veya kontrol adi (orn. "host3 SSH erisimi")
	OK     bool
	Detail string // basarisizsa neden, basariliysa (istege bagli) kisa bilgi
	Warn   bool   // true ise FAIL degil ama dikkat cekilmesi gereken bir durum (orn. opsiyonel host6 bos)
}

// reqVar, zorunlu bir ortam degiskenini ve kisa aciklamasini tanimlar.
type reqVar struct {
	name string
	desc string
}

// requiredVars, env.sh sablonunda "CHANGE_ME_" ile isaretli ve
// senkronun DOGRU calismasi icin zorunlu olan degiskenlerin listesidir.
// Sira, config/env.sh'deki sirayla ESLESIR -- rapor okunurken kafa
// karistirmasin diye.
var requiredVars = []reqVar{
	{"user", "SSH/isletim sistemi kullanicisi"},
	{"host0", "Ana Makine (Main/Control)"},
	{"host1", "Hedef Veritabani sunucusu"},
	{"host2", "Passport/Comment/Dashboard/Swym/FedSearch sunucusu"},
	{"host3", "3DSpace/3DSpaceInternal sunucusu"},
	{"host4", "3DSpaceIndex/3DIndex sunucusu"},
	{"host5", "FCS Central sunucusu"},
	{"host7", "Kaynak FCS sunucusu"},
	{"host8", "Kaynak Veritabani sunucusu"},
	{"host9", "FCS Remote sunucusu"},
	{"mainpath", "3dsync-agent deploy dizini"},
	{"exppath", "3DEXPERIENCE kurulum kok dizini"},
	{"sqlpath", "Hedef DB gecici dizini (host1)"},
	{"sqlpath2", "Kaynak DB gecici dizini (host8)"},
	{"sqldata", "SQL Server data dizini (host1)"},
	{"sqllog", "SQL Server log dizini (host1)"},
	{"DB_NAME", "Veritabani adi"},
	{"dest1", "FCS hedef kok dizini (host3)"},
	{"src1", "FCS kaynak kok dizini (host7)"},
	{"urlconf1", "config.xml icinde aranan metin"},
	{"urlconf2", "config.xml icine yazilacak metin"},
	{"plmxurl", "STORE plmx hedef URL"},
	{"storeurl", "STORE store hedef URL"},
	{"imageurl", "STORE imagestore hedef URL"},
}

// optionalVars: bos birakilmasi GECERLI olan, ama bos oldugunda bir
// UYARI (FAIL degil) olarak bildirilmesi faydali degiskenler.
var optionalVars = []reqVar{
	{"host6", "HAProxy/LB sunucusu -- yoksa bos birakilabilir (araclar otomatik atlar)"},
	{"password", "SQL Server 'sa' parolasi (config/secrets.sh icinde olmali)"},
}

// placeholderMarker, sablonda hala degistirilmemis yer tutuculari
// yakalamak icin aranan alt-metin.
const placeholderMarker = "CHANGE_ME"

// CheckEnv, yukaridaki zorunlu degiskenleri os.Getenv ile okuyup
// bos/placeholder durumlarini raporlar. SSH baglanti kontrolu YAPMAZ
// (bkz. CheckSSH) -- bu sadece env.sh'in kendi icerigini dogrular.
func CheckEnv() []Result {
	var results []Result
	for _, v := range requiredVars {
		val := os.Getenv(v.name)
		switch {
		case val == "":
			results = append(results, Result{Name: v.name, OK: false,
				Detail: fmt.Sprintf("BOS -- %s icin bir deger yok", v.desc)})
		case strings.Contains(val, placeholderMarker):
			results = append(results, Result{Name: v.name, OK: false,
				Detail: fmt.Sprintf("HALA SABLON DEGERI ('%s') -- %s icin gercek deger girilmedi", val, v.desc)})
		default:
			results = append(results, Result{Name: v.name, OK: true, Detail: val})
		}
	}
	for _, v := range optionalVars {
		val := os.Getenv(v.name)
		switch {
		case val == "":
			results = append(results, Result{Name: v.name, OK: true, Warn: true,
				Detail: fmt.Sprintf("bos (opsiyonel) -- %s", v.desc)})
		case strings.Contains(val, placeholderMarker):
			results = append(results, Result{Name: v.name, OK: false,
				Detail: fmt.Sprintf("HALA SABLON DEGERI ('%s') -- %s icin gercek deger girilmedi", val, v.desc)})
		default:
			results = append(results, Result{Name: v.name, OK: true, Detail: "doldurulmus"})
		}
	}
	return results
}

// CheckSSH, host0..host9 icinde DOLU olan (ve placeholder OLMAYAN) her
// BENZERSIZ host degerine, sshUser ile parolasiz (BatchMode) SSH
// baglanabildigini test eder. timeoutSec, her deneme icin ssh
// ConnectTimeout degeridir.
func CheckSSH(sshUser string, timeoutSec int) []Result {
	var results []Result
	seen := map[string]bool{}
	hostVars := []string{"host0", "host1", "host2", "host3", "host4", "host5", "host6", "host7", "host8", "host9"}

	for _, hv := range hostVars {
		val := os.Getenv(hv)
		if val == "" || strings.Contains(val, placeholderMarker) {
			continue // CheckEnv zaten bunu bildirdi, SSH denemeye deger yok
		}
		if seen[val] {
			continue // ayni host birden fazla rolde (tek sunuculu kurulum) -- bir kez test etmek yeterli
		}
		seen[val] = true

		target := fmt.Sprintf("%s@%s", sshUser, val)
		cmd := exec.Command("ssh",
			"-o", "BatchMode=yes",
			"-o", fmt.Sprintf("ConnectTimeout=%d", timeoutSec),
			"-o", "StrictHostKeyChecking=accept-new",
			target, "true")
		out, err := cmd.CombinedOutput()
		if err != nil {
			detail := strings.TrimSpace(string(out))
			if detail == "" {
				detail = err.Error()
			}
			results = append(results, Result{
				Name: fmt.Sprintf("%s (%s) SSH erisimi", hv, val), OK: false,
				Detail: detail,
			})
		} else {
			results = append(results, Result{
				Name: fmt.Sprintf("%s (%s) SSH erisimi", hv, val), OK: true,
				Detail: "sifresiz baglanti basarili",
			})
		}
	}

	// Raporu host adina gore stabil sirala (map/seen sirasi degisken olabilir).
	sort.SliceStable(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results
}

// AnyFailed, Result listesinde en az bir basarisiz (OK=false) sonuc
// olup olmadigini dondurur -- Print()'in yan etkisiz (stdout'a
// yazmayan) cekirdegi, testlerde ayrica kullanilabilir.
func AnyFailed(results []Result) bool {
	for _, r := range results {
		if !r.OK {
			return true
		}
	}
	return false
}

// Print, bir Result listesini okunakli bir rapor olarak basar ve
// icinde en az bir basarisiz (Warn=false, OK=false) sonuc olup
// olmadigini dondurur.
func Print(title string, results []Result) (hasFailure bool) {
	fmt.Println("== " + title + " ==")
	for _, r := range results {
		switch {
		case r.OK && r.Warn:
			fmt.Printf("  [UYARI]  %-45s %s\n", r.Name, r.Detail)
		case r.OK:
			fmt.Printf("  [OK]     %-45s %s\n", r.Name, r.Detail)
		default:
			fmt.Printf("  [HATA]   %-45s %s\n", r.Name, r.Detail)
		}
	}
	fmt.Println()
	return AnyFailed(results)
}
