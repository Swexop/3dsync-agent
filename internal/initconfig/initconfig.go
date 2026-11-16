// Package initconfig, "3dsync-agent init-config" alt-komutunun mantigini
// barindirir: config/env.sh SABLONUNDAKI "CHANGE_ME_..." yer tutucularini,
// dosyanin YAPISINI (yorumlar, blok isaretleri, zaten-dogru turetilmis
// satirlar: orn. src2="$sqlpath2/...") KORUYARAK, interaktif sorularla
// VEYA bir "answers" (cevap) dosyasiyla doldurur.
//
// AMAC (urunlestirme, "cok sayida tekrarlayan kurulum"): env.sh'i her
// musteride elle 80+ satir duzenlemek -- tek bir degeri atlamak/yanlis
// satira yazmak COK kolay (bkz. internal/preflight, bu hatalari
// dagitimdan SONRA yakalamak icin yazildi). init-config, aynı hatalari
// dagitimdan ONCE, sistematik bir soru-cevapla ONLEMEYI hedefler:
//   - Interaktif modda: her yer tutucu icin, dosyadaki yorumu (aciklamayi)
//     GOSTERIR, sonra deger sorar.
//   - "--answers <dosya>" ile: KEY=VALUE formatinda bir cevap dosyasindan
//     degerleri OKUR (orn. bir onceki musteriden kopyalanan, duzenlenmis
//     bir sablon) -- cok sayida kurulumda ayni sorulari tekrar tekrar
//     elle cevaplamak yerine, scriptlenebilir bir akis saglar.
package initconfig

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// placeholder, tek bir "CHANGE_ME_..." satirinin ayristirilmis halidir.
type placeholder struct {
	lineIdx  int // outLines icindeki satir index'i (yerine koyulacak)
	varName  string
	quoted   bool   // "NAME=\"CHANGE_ME_...\"" mi yoksa "NAME=CHANGE_ME_..." mi
	trailing string // unquoted durumda satirin sonundaki " # yorum" kismi (varsa)
	example  string // CHANGE_ME_ sonrasi orijinal metin -- beklenen SEKIL icin ipucu
	desc     []string
}

var (
	quotedRe   = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)="CHANGE_ME_(.*)"$`)
	unquotedRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=CHANGE_ME_(\S+)(\s*#.*)?$`)
)

// Options, Run()'a verilen ayarlardir.
type Options struct {
	TemplatePath   string
	OutputPath     string // bos ise TemplatePath ile ayni (yerinde guncelleme + .bak yedegi)
	AnswersPath    string // bos ise sadece interaktif
	NonInteractive bool   // true ise, answers'ta olmayan her degisken HATA sayilir
	In             io.Reader
	Out            io.Writer
}

// Run, sablonu okur, yer tutuculari doldurur, cikis dosyasini yazar.
// Donen hata, kullaniciya dogrudan gosterilecek bir mesaj TASIYABILIR
// (fmt.Errorf ile zaten anlasilir metin uretiliyor).
func Run(opt Options) error {
	if opt.OutputPath == "" {
		opt.OutputPath = opt.TemplatePath
	}
	if opt.In == nil {
		opt.In = os.Stdin
	}
	if opt.Out == nil {
		opt.Out = os.Stdout
	}

	raw, err := os.ReadFile(opt.TemplatePath)
	if err != nil {
		return fmt.Errorf("sablon okunamadi (%s): %w", opt.TemplatePath, err)
	}
	lines := strings.Split(string(raw), "\n")

	answers := map[string]string{}
	if opt.AnswersPath != "" {
		answers, err = readAnswers(opt.AnswersPath)
		if err != nil {
			return fmt.Errorf("answers dosyasi okunamadi (%s): %w", opt.AnswersPath, err)
		}
	}

	var pending []placeholder
	var descBuf []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#") {
			if strings.Contains(trimmed, "===") || trimmed == "#" || isHashOnly(trimmed) {
				// Yapisal ayrac/blok isareti -- aciklama metnine KATMA, ama
				// dosyada DEGISTIRMEDEN kalacak (zaten pass-through).
				continue
			}
			descBuf = append(descBuf, strings.TrimSpace(strings.TrimPrefix(trimmed, "#")))
			continue
		}

		if trimmed == "" {
			descBuf = nil
			continue
		}

		if m := quotedRe.FindStringSubmatch(trimmed); m != nil {
			pending = append(pending, placeholder{
				lineIdx: i, varName: m[1], quoted: true, example: m[2],
				desc: append([]string(nil), descBuf...),
			})
			descBuf = nil
			continue
		}
		if m := unquotedRe.FindStringSubmatch(trimmed); m != nil {
			pending = append(pending, placeholder{
				lineIdx: i, varName: m[1], quoted: false, example: m[2], trailing: m[3],
				desc: append([]string(nil), descBuf...),
			})
			descBuf = nil
			continue
		}
		descBuf = nil
	}

	if len(pending) == 0 {
		fmt.Fprintln(opt.Out, "Bu sablonda doldurulacak 'CHANGE_ME_' yer tutucusu kalmamis -- yapilacak bir sey yok.")
		return nil
	}

	reader := bufio.NewReader(opt.In)
	var missing []string

	for idx, ph := range pending {
		val, fromAnswers := answers[ph.varName]
		if fromAnswers {
			fmt.Fprintf(opt.Out, "[%d/%d] %-18s <- answers dosyasindan: %s\n", idx+1, len(pending), ph.varName, val)
		} else {
			if opt.NonInteractive {
				missing = append(missing, ph.varName)
				continue
			}
			val = prompt(opt.Out, reader, ph, idx+1, len(pending))
			if val == "" {
				missing = append(missing, ph.varName)
				continue
			}
		}
		lines[ph.lineIdx] = renderLine(ph, val)
	}

	if len(missing) > 0 {
		return fmt.Errorf("su degiskenler doldurulmadi (atlandi/answers'ta yok): %s -- cikis dosyasi yazilmadi", strings.Join(missing, ", "))
	}

	result := strings.Join(lines, "\n")

	// Guvenlik kontrolu: yazmadan once sozdizimini dogrula (bash -n).
	// Kullanicinin girdigi bir deger (orn. icinde " karakteri olan bir
	// yol) satiri bozmus olabilir -- bunu DOSYAYI EZMEDEN yakalamak icin.
	tmp, err := os.CreateTemp("", "env.sh.check.*")
	if err != nil {
		return fmt.Errorf("gecici dogrulama dosyasi olusturulamadi: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(result); err != nil {
		tmp.Close()
		return fmt.Errorf("gecici dogrulama dosyasina yazilamadi: %w", err)
	}
	tmp.Close()
	if out, err := exec.Command("bash", "-n", tmp.Name()).CombinedOutput(); err != nil {
		return fmt.Errorf("uretilen env.sh GECERSIZ bash sozdizimi iceriyor (girilen bir deger satiri bozmus olabilir) -- dosya YAZILMADI:\n%s", strings.TrimSpace(string(out)))
	}

	if opt.OutputPath == opt.TemplatePath {
		backup := fmt.Sprintf("%s.bak.%s", opt.TemplatePath, time.Now().Format("20060102150405"))
		if err := copyFile(opt.TemplatePath, backup); err != nil {
			return fmt.Errorf("yedek alinamadi (%s): %w", backup, err)
		}
		fmt.Fprintf(opt.Out, "Eski hali yedeklendi: %s\n", backup)
	}

	if err := os.WriteFile(opt.OutputPath, []byte(result), 0644); err != nil {
		return fmt.Errorf("cikis dosyasi yazilamadi (%s): %w", opt.OutputPath, err)
	}

	fmt.Fprintf(opt.Out, "\nTAMAMLANDI: %d deger dolduruldu, yazildi: %s\n", len(pending), opt.OutputPath)
	fmt.Fprintln(opt.Out, "Sonraki adim: ./3dsync-agent validate-config --skip-ssh  (sonra, SSH anahtarlari kurulduktan sonra --skip-ssh'siz tekrar)")
	return nil
}

func isHashOnly(s string) bool {
	for _, c := range s {
		if c != '#' {
			return false
		}
	}
	return true
}

func prompt(out io.Writer, reader *bufio.Reader, ph placeholder, n, total int) string {
	fmt.Fprintf(out, "\n--- [%d/%d] %s ---\n", n, total, ph.varName)
	for _, d := range ph.desc {
		fmt.Fprintf(out, "  %s\n", d)
	}
	fmt.Fprintf(out, "  (ornek sekil: %s)\n", ph.example)
	fmt.Fprint(out, "Deger (bos birakip Enter: atla/sonra tekrar calistir): ")
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func renderLine(ph placeholder, val string) string {
	if ph.quoted {
		return fmt.Sprintf("%s=%q", ph.varName, val)
	}
	return fmt.Sprintf("%s=%s%s", ph.varName, val, ph.trailing)
}

// readAnswers, basit bir "KEY=VALUE" dosyasini okur -- bash'in kendisi
// DEGIL, sadece satir satir ayristirilir (secrets icermemesi gerekir,
// bu dosya bash olarak source EDILMEZ). "#" ile baslayan ve bos
// satirlar atlanir. Deger tirnak icindeyse tirnaklar soyulur.
func readAnswers(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return nil, fmt.Errorf("satir %d: '=' bulunamadi: %q", lineNo, line)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		out[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0644)
}
