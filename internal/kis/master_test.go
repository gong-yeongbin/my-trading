package kis

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/text/encoding/korean"
)

const (
	kospiSamsung  = `005930   KR7005930003삼성전자                                ST1002700130000 NN5YYY YYNNNNNNN0NNNNNNNY0002690000000100001NNN00NNN000000020Y0900000225170760000000001001975061100000000584627800000000077804668500012       0 NYY00305372900146725200153268211884000031.3920260630015726489511NNN`
	kospiManaged  = `000040   KR7000040006KR모터스                                ST3002700150000 NN0NNN NNNNNNNNN0NNNNNNNN0000011510000100001NNY00NNN000000100N0900000000299950000000025001976052500000000001967500000000004918759000012       0 NNY000000289-0000000200000028300289000054.8320260630000000226   NNN`
	kospiDLPref   = `000215   KR7000211003DL우                                    ST0002700080000 NN0NNN NNNNNNNNN0NNNNNNNN0000254000000100001NNN00NNN000000100N0900000000020040000000050001989082100000000000168600000000001043057500012       1 NNN00000000000000000000000000000000000000.00        000000428   NNN`
	kosdaqEcopro  = `247540   KR7247540008에코프로비엠                            ST1100910280000 NY Y    N  0  N    Y0001039000000100001NNN00NNN000000040Y0900000003787260000000005002019030500000000009783000000000004895545400012       0 NY0000118210000003900000001420011700000000320260630000101645LD2NNN`
	kosdaqSpac    = `0004Y0   KR70004Y0000디비금융제14호스팩                      ST3101400000000 NN N    Y  0  N    N0000019880000100001NNN00NNN000000100N0900000000042230000000001002025072200000000000531500000000000053150000012       0 NN0000000000000000000000000000000000000000020241231000000105   NNN`
	kosdaqManaged = `001000   KR7001000009신라섬유                                ST3000000000000NNN NNNNNNNN0NNNNNNNN0000017720000100001NNY00NNN000000100N0900000002136620000000005001994062800000000000485500000000000242775400012       0 NN000000037000000004-00000003-0003-0000000220241231000000086   NNN`
)

func cp949(t *testing.T, lines ...string) []byte {
	t.Helper()
	b, err := korean.EUCKR.NewEncoder().Bytes([]byte(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseMasterKospi(t *testing.T) {
	rows, err := ParseMaster(bytes.NewReader(cp949(t, kospiSamsung, kospiManaged, kospiDLPref)), "kospi")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	if pf := rows[2]; pf.Code != "000215" || pf.Name != "DL우" || !pf.Preferred || pf.Managed {
		t.Errorf("DL우 = %+v, want Preferred", pf)
	}
	if on, err := rows[2].FlagOn("우선주"); err != nil || !on {
		t.Errorf("우선주 flag = %v err=%v", on, err)
	}
	s := rows[0]
	if s.Code != "005930" || s.Name != "삼성전자" || s.Market != "kospi" || s.GroupCode != "ST" {
		t.Errorf("samsung = %+v", s)
	}
	for _, flag := range KnownFlags {
		if on, err := s.FlagOn(flag); err != nil || on {
			t.Errorf("samsung flag %s = %v err=%v, want off", flag, on, err)
		}
	}
	m := rows[1]
	if m.Code != "000040" || m.Name != "KR모터스" || !m.Managed {
		t.Errorf("managed = %+v", m)
	}
	if on, _ := m.FlagOn("관리종목"); !on {
		t.Error("관리종목 flag should be on")
	}
	if on, _ := m.FlagOn("거래정지"); on {
		t.Error("거래정지 should be off")
	}
}

func TestParseMasterKosdaq(t *testing.T) {
	rows, err := ParseMaster(bytes.NewReader(cp949(t, kosdaqEcopro, kosdaqSpac, kosdaqManaged)), "kosdaq")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	if e := rows[0]; e.Code != "247540" || e.Name != "에코프로비엠" || e.GroupCode != "ST" || e.Spac || e.Managed {
		t.Errorf("ecopro = %+v", e)
	}
	if sp := rows[1]; sp.Code != "0004Y0" || !sp.Spac {
		t.Errorf("spac = %+v", sp)
	}
	if mg := rows[2]; mg.Code != "001000" || !mg.Managed || mg.WarningCode != "00" {
		t.Errorf("managed = %+v", mg)
	}
}

func TestFlagOnUnknown(t *testing.T) {
	if _, err := (MasterRow{}).FlagOn("없는플래그"); err == nil {
		t.Error("expected error for unknown flag")
	}
	r := MasterRow{WarningCode: "02", OverheatCode: "1"}
	if on, _ := r.FlagOn("시장경고"); !on {
		t.Error("시장경고 02 should be on")
	}
	if on, _ := r.FlagOn("단기과열"); !on {
		t.Error("단기과열 1 should be on")
	}
}

func TestParseMasterRejectsShortLine(t *testing.T) {
	if _, err := ParseMaster(bytes.NewReader(cp949(t, "too short")), "kospi"); err == nil {
		t.Error("expected error")
	}
}

func TestDownloadMaster(t *testing.T) {
	f := newFakeKIS(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("kosdaq_code.mst")
	w.Write(cp949(t, kosdaqEcopro))
	zw.Close()
	f.handle("/common/master/kosdaq_code.mst.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf.Bytes())
	})
	f.client.MasterBaseURL = f.srv.URL
	rows, err := f.client.DownloadMaster(context.Background(), "kosdaq")
	if err != nil || len(rows) != 1 || rows[0].Code != "247540" {
		t.Errorf("rows=%+v err=%v", rows, err)
	}
	if f.calls() != 0 {
		t.Error("master download must not issue a token")
	}
	if _, err := f.client.DownloadMaster(context.Background(), "nasdaq"); err == nil {
		t.Error("unknown market should error")
	}
}
