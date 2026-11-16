package fcssync

import "testing"

func TestCountChangesFromLines(t *testing.T) {
	lines := []string{
		">f+++++++++ a/b.txt",
		"cd+++++++++ a/dir/",
		"hf          a/hardlink.txt",
		"*deleting   a/old.txt",
		"*deleting   a/old2.txt",
		"this is not a change line",
		"",
	}
	changed, deleted := CountChangesFromLines(lines)
	if changed != 3 {
		t.Errorf("changed = %d, beklenen 3", changed)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, beklenen 2", deleted)
	}
}

func TestCountChangesFromLines_Empty(t *testing.T) {
	changed, deleted := CountChangesFromLines(nil)
	if changed != 0 || deleted != 0 {
		t.Errorf("bos girdide changed=%d deleted=%d, ikisi de 0 olmali", changed, deleted)
	}
}

func TestCountChangesFromLines_NoMatches(t *testing.T) {
	lines := []string{"receiving file list ... done", "sent 123 bytes  received 45 bytes"}
	changed, deleted := CountChangesFromLines(lines)
	if changed != 0 || deleted != 0 {
		t.Errorf("eslesmeyen satirlarda changed=%d deleted=%d, ikisi de 0 olmali", changed, deleted)
	}
}
