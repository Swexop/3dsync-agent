package postsync

import "testing"

func TestBuildJobs_CleanupBeforeStartup_SameHost(t *testing.T) {
	h := Hosts{
		Host2:            "h2",
		CommentTomeePath: "/opt/comment",
	}
	jobs := BuildJobs(h)
	cleanupIdx, startupIdx := -1, -1
	for i, j := range jobs {
		if j.Host != "h2" {
			continue
		}
		if j.Cmd == "rm -rf /opt/comment/logs/*" && cleanupIdx == -1 {
			cleanupIdx = i
		}
		if j.Cmd == "/opt/comment/bin/startup.sh" {
			startupIdx = i
		}
	}
	if cleanupIdx == -1 || startupIdx == -1 {
		t.Fatalf("beklenen temizlik/baslatma komutlari bulunamadi: %+v", jobs)
	}
	if cleanupIdx >= startupIdx {
		t.Errorf("temizlik (idx=%d) baslatmadan (idx=%d) ONCE gelmeli", cleanupIdx, startupIdx)
	}
}

func TestBuildJobs_EmptyPath_SkipsRole(t *testing.T) {
	h := Hosts{Host2: "h2", CommentTomeePath: ""}
	jobs := BuildJobs(h)
	for _, j := range jobs {
		if j.Host == "h2" {
			t.Errorf("CommentTomeePath bos iken h2 icin hicbir is kuyruklanmamali, bulundu: %+v", j)
		}
	}
}

func TestBuildJobs_Host9Optional(t *testing.T) {
	without := BuildJobs(Hosts{Host5: "h5", FCSCentralPath: "/opt/fcs"})
	for _, j := range without {
		if j.Host == "" {
			t.Errorf("bos host ile is kuyruklanmamali")
		}
	}

	with := BuildJobs(Hosts{
		Host5: "h5", FCSCentralPath: "/opt/fcs",
		Host9: "h9", FCSRemotePath: "/opt/fcsremote",
	})
	found := false
	for _, j := range with {
		if j.Host == "h9" {
			found = true
		}
	}
	if !found {
		t.Error("host9 verildiginde FCS Remote icin is kuyruklanmali")
	}
	if len(with) <= len(without) {
		t.Error("host9 ekleyince toplam is sayisi artmali")
	}
}

func TestBuildJobs_LBOnlyWhenHost6Set(t *testing.T) {
	without := BuildJobs(Hosts{})
	for _, j := range without {
		if j.Cmd == lbReloadCmd {
			t.Error("Host6 bos iken LB reload kuyruklanmamali")
		}
	}

	with := BuildJobs(Hosts{Host6: "h6"})
	found := false
	for _, j := range with {
		if j.Host == "h6" && j.Cmd == lbReloadCmd {
			found = true
		}
	}
	if !found {
		t.Error("Host6 verildiginde LB reload komutu kuyruklanmali")
	}
}

func TestBuildJobs_ExpPathDependentJobs(t *testing.T) {
	without := BuildJobs(Hosts{Host2: "h2", Host3: "h3", Host4: "h4"})
	if len(without) != 0 {
		t.Errorf("ExpPath ve tum tomeePath'ler bos iken hicbir is olmamali, bulundu: %+v", without)
	}

	with := BuildJobs(Hosts{Host2: "h2", Host3: "h3", Host4: "h4", ExpPath: "/exp"})
	wantHosts := map[string]bool{"h2": false, "h3": false, "h4": false}
	for _, j := range with {
		wantHosts[j.Host] = true
	}
	for host, seen := range wantHosts {
		if !seen {
			t.Errorf("ExpPath verildiginde %s icin en az bir is beklenirdi", host)
		}
	}
}
