package configgen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerate_SingleReplacement(t *testing.T) {
	dir := t.TempDir()
	syncResult := filepath.Join(dir, "SyncResult")
	if err := os.MkdirAll(syncResult, 0755); err != nil {
		t.Fatal(err)
	}
	before := `<config><url>https://3dspacefts.example.com/index</url></config>`
	if err := os.WriteFile(filepath.Join(syncResult, "config_Before.xml"), []byte(before), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		MainPath: dir,
		URLConf1: "3dspacefts",
		URLConf2: "3dspaceftsqa",
	}

	applied, err := cfg.Generate()
	if err != nil {
		t.Fatalf("Generate hata verdi: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, beklenen 1", applied)
	}

	got, err := os.ReadFile(filepath.Join(syncResult, "config.xml"))
	if err != nil {
		t.Fatal(err)
	}
	want := `<config><url>https://3dspaceftsqa.example.com/index</url></config>`
	if string(got) != want {
		t.Fatalf("config.xml =\n%s\nbeklenen:\n%s", got, want)
	}
}

func TestGenerate_NoReplacementWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	syncResult := filepath.Join(dir, "SyncResult")
	os.MkdirAll(syncResult, 0755)
	before := `<config><url>unchanged</url></config>`
	os.WriteFile(filepath.Join(syncResult, "config_Before.xml"), []byte(before), 0644)

	cfg := Config{MainPath: dir}
	applied, err := cfg.Generate()
	if err != nil {
		t.Fatalf("Generate hata verdi: %v", err)
	}
	if applied != 0 {
		t.Fatalf("applied = %d, beklenen 0", applied)
	}
	got, _ := os.ReadFile(filepath.Join(syncResult, "config.xml"))
	if string(got) != before {
		t.Fatalf("degisiklik yapilmamasi gerekirken icerik degisti: %s", got)
	}
}

func TestGenerate_MissingBeforeFile(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{MainPath: dir, URLConf1: "a", URLConf2: "b"}
	if _, err := cfg.Generate(); err == nil {
		t.Fatal("config_Before.xml yokken hata beklenirdi")
	}
}
