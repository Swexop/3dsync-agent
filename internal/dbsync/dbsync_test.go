package dbsync

import (
	"strings"
	"testing"
)

func TestBuildMoveClauses_DataAndLog(t *testing.T) {
	// RESTORE FILELISTONLY-benzeri cikti: LogicalName|PhysicalName|Type|...
	fileList := "spacedb        |/sqldata/spacedb.mdf|D|...\nspacedb_log    |/sqldata/spacedb_log.ldf|L|...\n"

	clauses, count, err := BuildMoveClauses(fileList, "spacedb", "/newdata", "/newlog")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, beklenen 2", count)
	}
	if !strings.Contains(clauses, "MOVE 'spacedb' TO '/newdata/spacedb.mdf'") {
		t.Fatalf("data move ifadesi eksik: %s", clauses)
	}
	// Ilk log dosyasi DB adiyla adlandirilmali (spacedb.ldf), mantiksal
	// adiyla (spacedb_log.ldf) DEGIL.
	if !strings.Contains(clauses, "MOVE 'spacedb_log' TO '/newlog/spacedb.ldf'") {
		t.Fatalf("log move ifadesi beklenen adlandirmada degil: %s", clauses)
	}
}

func TestBuildMoveClauses_MultipleLogFiles(t *testing.T) {
	fileList := "db      |/x/db.mdf|D|\nlog1    |/x/log1.ldf|L|\nlog2    |/x/log2.ldf|L|\n"
	clauses, count, err := BuildMoveClauses(fileList, "db", "/data", "/log")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, beklenen 3", count)
	}
	// Ilk log -> DB adi, ikinci log -> kendi mantiksal adi (cakisma onlenir)
	if !strings.Contains(clauses, "MOVE 'log1' TO '/log/db.ldf'") {
		t.Fatalf("ilk log dosyasi DB adiyla adlandirilmamis: %s", clauses)
	}
	if !strings.Contains(clauses, "MOVE 'log2' TO '/log/log2.ldf'") {
		t.Fatalf("ikinci log dosyasi kendi adiyla kalmamis: %s", clauses)
	}
}

func TestBuildMoveClauses_NoValidFiles(t *testing.T) {
	_, _, err := BuildMoveClauses("", "db", "/data", "/log")
	if err == nil {
		t.Fatal("bos filelist icin hata beklenirdi")
	}
}

func TestBuildMoveClauses_UnknownTypeSkipped(t *testing.T) {
	fileList := "db   |/x/db.mdf|D|\nfts  |/x/fts.dat|F|\n"
	clauses, count, err := BuildMoveClauses(fileList, "db", "/data", "/log")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, beklenen 1 (taninmayan tur atlanmali)", count)
	}
	if strings.Contains(clauses, "fts") {
		t.Fatalf("taninmayan turdeki dosya MOVE ifadesine girmemeliydi: %s", clauses)
	}
}
