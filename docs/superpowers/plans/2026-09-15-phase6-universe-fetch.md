# 6단계: 유니버스·일봉 수집 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 한투 종목 마스터 파일로 유니버스를 구성하고, 종목·지수 일봉을 증분 수집하는 `trader universe`, `trader fetch` 서브커맨드를 완성한다. 이 단계가 끝나면 SQLite에 실데이터가 쌓인다.

**Architecture:** `kis`(마스터 파일) → `app`(universe·fetch 오케스트레이션, 인터페이스로 kis를 주입) → `cmd/trader`(서브커맨드 추가). 실서버는 마지막 수동 확인에서만 호출한다.

**Tech Stack:** Go 1.27, `golang.org/x/text`(CP949 디코딩). 테스트는 표준 `testing` + `httptest`.

**Spec:** `docs/superpowers/specs/2026-09-13-backtest-design.md` (4절, 7.5절, 11절의 fetch 항목, 12절의 kis 마스터 항목)

**선행:** 5단계(한투 클라이언트) 완료.

## Global Constraints

- 모듈 경로: `github.com/gong-yeongbin/my-trading`
- 외부 의존성은 위 3개로 제한. 테스트 프레임워크(testify 등) 추가 금지.
- 가격은 `int64` 원 단위, 지수는 `float64`. 날짜는 `YYYY-MM-DD` 문자열로 저장하고 `time.Time`은 KST 자정.
- 한투 실서버는 테스트에서 호출하지 않는다. 마지막 수동 확인에서만 호출.
- **git 명령은 이 프로젝트에서 Claude에게 차단되어 있다.** "커밋" 단계는 명령을 출력해 사용자에게 실행을 요청하는 것으로 대체한다.
- `.env`, `data/token.json`은 읽기 금지. 값이 필요하면 프로그램이 읽게 하고 Claude는 열지 않는다.
- 파일 편집 후 `gofmt -l .` 결과가 비어 있어야 한다.

---


## 파일 구조

```
internal/kis/master.go        마스터 파일 다운로드·파싱
internal/kis/master_test.go
internal/app/universe.go      RunUniverse
internal/app/fetch.go         RunFetch
internal/app/universe_test.go
internal/app/fetch_test.go
cmd/trader/main.go            universe, fetch 서브커맨드 추가 (수정)
```

---

### Task 1: 종목 마스터 파일 파싱과 다운로드

**Files:**
- Create: `internal/kis/master.go`
- Test: `internal/kis/master_test.go`

**Interfaces:**
- Produces: `kis.MasterRow` 구조체, `(MasterRow).FlagOn(flag string) (bool, error)`, `kis.ParseMaster(r io.Reader, market string) ([]MasterRow, error)`(CP949 입력), `(*Client).DownloadMaster(ctx, market string) ([]MasterRow, error)`, `kis.MasterURL(market string) string`, `kis.KnownFlags []string`.
- 파일 형식 (공식 예제 `stocks_info/kis_kospi_code_mst.py`, `kis_kosdaq_code_mst.py`에서 확인): 한 줄이 한 종목. 앞부분 = 단축코드 9자 + 표준코드 12자 + 한글명(가변). 뒷부분 = 코스피 228자 / 코스닥 222자 고정폭. **뒷부분 첫 글자는 공백이고 두 번째 글자부터 폭 배열이 적용된다** (폭 합계가 227/221인 이유). 실제 파일에서 검증한 정상값: 거래정지·정리매매·관리종목·이상급등·SPAC = `N`, 시장경고 = `00`, 단기과열 = `0`, 그룹코드 보통주 = `ST`.

- [ ] **Step 1: 의존성 추가**

```bash
go get golang.org/x/text@latest
```

- [ ] **Step 2: 실패하는 테스트 작성**

`internal/kis/master_test.go` (아래 픽스처는 2026-09 실제 마스터 파일에서 그대로 가져온 줄이다. 공백 개수까지 정확해야 하므로 수정하지 말 것):

```go
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
	kospiSamsung = `005930   KR7005930003삼성전자                                ST1002700130000 NN5YYY YYNNNNNNN0NNNNNNNY0002690000000100001NNN00NNN000000020Y0900000225170760000000001001975061100000000584627800000000077804668500012       0 NYY00305372900146725200153268211884000031.3920260630015726489511NNN`
	kospiManaged = `000040   KR7000040006KR모터스                                ST3002700150000 NN0NNN NNNNNNNNN0NNNNNNNN0000011510000100001NNY00NNN000000100N0900000000299950000000025001976052500000000001967500000000004918759000012       0 NNY000000289-0000000200000028300289000054.8320260630000000226   NNN`
	kosdaqEcopro = `247540   KR7247540008에코프로비엠                            ST1100910280000 NY Y    N  0  N    Y0001039000000100001NNN00NNN000000040Y0900000003787260000000005002019030500000000009783000000000004895545400012       0 NY0000118210000003900000001420011700000000320260630000101645LD2NNN`
	kosdaqSpac   = `0004Y0   KR70004Y0000디비금융제14호스팩                      ST3101400000000 NN N    Y  0  N    N0000019880000100001NNN00NNN000000100N0900000000042230000000001002025072200000000000531500000000000053150000012       0 NN0000000000000000000000000000000000000000020241231000000105   NNN`
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
	rows, err := ParseMaster(bytes.NewReader(cp949(t, kospiSamsung, kospiManaged)), "kospi")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
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
```

- [ ] **Step 3: 테스트 실패 확인**

Run: `go test ./internal/kis/ -run Master 2>&1 | head -5`
Expected: 컴파일 실패

- [ ] **Step 4: 구현**

`internal/kis/master.go`:

```go
package kis

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/text/encoding/korean"
)

const DefaultMasterBaseURL = "https://new.real.download.dws.co.kr"

// KnownFlags 는 config universe.exclude_flags 에 쓸 수 있는 이름이다.
var KnownFlags = []string{"거래정지", "정리매매", "관리종목", "시장경고", "단기과열", "이상급등", "SPAC"}

type MasterRow struct {
	Code         string
	Name         string
	Market       string // kospi | kosdaq
	GroupCode    string // ST=보통주, EF=ETF, ...
	Suspended    bool   // 거래정지
	Liquidation  bool   // 정리매매
	Managed      bool   // 관리종목
	WarningCode  string // 시장경고: 00 정상, 01 투자주의 02 경고 03 위험
	OverheatCode string // 단기과열: 0 정상
	Surge        bool   // 이상급등
	Spac         bool
}

func (r MasterRow) FlagOn(flag string) (bool, error) {
	switch flag {
	case "거래정지":
		return r.Suspended, nil
	case "정리매매":
		return r.Liquidation, nil
	case "관리종목":
		return r.Managed, nil
	case "시장경고":
		return r.WarningCode != "00", nil
	case "단기과열":
		return r.OverheatCode != "0", nil
	case "이상급등":
		return r.Surge, nil
	case "SPAC":
		return r.Spac, nil
	}
	return false, fmt.Errorf("kis: unknown flag %q (known: %s)", flag, strings.Join(KnownFlags, ", "))
}

// 고정폭 정의. 공식 예제 stocks_info/kis_kospi_code_mst.py, kis_kosdaq_code_mst.py 의 field_specs 와 같다.
// 컬럼명은 두 시장에서 같은 뜻이면 같은 이름을 쓴다.
type masterLayout struct {
	tail   int // 줄 끝에서 고정폭 영역 길이
	widths []int
	cols   []string
}

var kospiLayout = masterLayout{
	tail: 228,
	widths: []int{2, 1, 4, 4, 4, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 9, 5, 5, 1, 1, 1, 2, 1, 1,
		1, 2, 2, 2, 3, 1, 3, 12, 12, 8, 15, 21, 2, 7, 1, 1, 1, 1, 1, 9, 9, 9, 5, 9, 8, 9, 3, 1, 1, 1},
	cols: []string{"그룹코드", "시가총액규모", "지수업종대분류", "지수업종중분류", "지수업종소분류", "제조업", "저유동성", "지배구조지수종목", "KOSPI200섹터업종", "KOSPI100",
		"KOSPI50", "KRX", "ETP", "ELW발행", "KRX100", "KRX자동차", "KRX반도체", "KRX바이오", "KRX은행", "SPAC",
		"KRX에너지화학", "KRX철강", "단기과열", "KRX미디어통신", "KRX건설", "Non1", "KRX증권", "KRX선박", "KRX섹터_보험", "KRX섹터_운송",
		"SRI", "기준가", "매매수량단위", "시간외수량단위", "거래정지", "정리매매", "관리종목", "시장경고", "경고예고", "불성실공시",
		"우회상장", "락구분", "액면변경", "증자구분", "증거금비율", "신용가능", "신용기간", "전일거래량", "액면가", "상장일자",
		"상장주수", "자본금", "결산월", "공모가", "우선주", "공매도과열", "이상급등", "KRX300", "KOSPI", "매출액",
		"영업이익", "경상이익", "당기순이익", "ROE", "기준년월", "시가총액", "그룹사코드", "회사신용한도초과", "담보대출가능", "대주가능"},
}

var kosdaqLayout = masterLayout{
	tail: 222,
	widths: []int{2, 1, 4, 4, 4, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 9, 5, 5, 1, 1, 1, 2, 1, 1, 1, 2, 2, 2, 3,
		1, 3, 12, 12, 8, 15, 21, 2, 7, 1, 1, 1, 1, 9, 9, 9, 5, 9, 8, 9, 3, 1, 1, 1},
	cols: []string{"그룹코드", "시가총액규모", "지수업종대분류", "지수업종중분류", "지수업종소분류", "벤처기업", "저유동성", "KRX", "ETP", "KRX100",
		"KRX자동차", "KRX반도체", "KRX바이오", "KRX은행", "SPAC", "KRX에너지화학", "KRX철강", "단기과열", "KRX미디어통신", "KRX건설",
		"투자주의환기", "KRX증권", "KRX선박", "KRX섹터_보험", "KRX섹터_운송", "KOSDAQ150", "기준가", "매매수량단위", "시간외수량단위", "거래정지",
		"정리매매", "관리종목", "시장경고", "경고예고", "불성실공시", "우회상장", "락구분", "액면변경", "증자구분", "증거금비율",
		"신용가능", "신용기간", "전일거래량", "액면가", "상장일자", "상장주수", "자본금", "결산월", "공모가", "우선주",
		"공매도과열", "이상급등", "KRX300", "매출액", "영업이익", "경상이익", "당기순이익", "ROE", "기준년월", "시가총액",
		"그룹사코드", "회사신용한도초과", "담보대출가능", "대주가능"},
}

var masterLayouts = map[string]masterLayout{"kospi": kospiLayout, "kosdaq": kosdaqLayout}

func MasterURL(market string) string {
	return "/common/master/" + market + "_code.mst.zip"
}

// ParseMaster 는 CP949 로 인코딩된 마스터 파일을 읽는다.
func ParseMaster(r io.Reader, market string) ([]MasterRow, error) {
	layout, ok := masterLayouts[market]
	if !ok {
		return nil, fmt.Errorf("kis: unknown market %q", market)
	}
	sc := bufio.NewScanner(korean.EUCKR.NewDecoder().Reader(r))
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	var rows []MasterRow
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		row, err := parseMasterLine(line, market, layout)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("kis: master read: %w", err)
	}
	return rows, nil
}

func parseMasterLine(line, market string, layout masterLayout) (MasterRow, error) {
	r := []rune(line)
	if len(r) < layout.tail+21 {
		return MasterRow{}, fmt.Errorf("kis: master line too short (%d runes): %q", len(r), line)
	}
	head, tail := r[:len(r)-layout.tail], r[len(r)-layout.tail:]
	fields := make(map[string]string, len(layout.cols))
	off := 1 // 고정폭 영역의 첫 글자는 공백
	for i, w := range layout.widths {
		fields[layout.cols[i]] = string(tail[off : off+w])
		off += w
	}
	return MasterRow{
		Code:         strings.TrimSpace(string(head[:9])),
		Name:         strings.TrimSpace(string(head[21:])),
		Market:       market,
		GroupCode:    strings.TrimSpace(fields["그룹코드"]),
		Suspended:    fields["거래정지"] == "Y",
		Liquidation:  fields["정리매매"] == "Y",
		Managed:      fields["관리종목"] == "Y",
		WarningCode:  fields["시장경고"],
		OverheatCode: fields["단기과열"],
		Surge:        fields["이상급등"] == "Y",
		Spac:         fields["SPAC"] == "Y",
	}, nil
}

// DownloadMaster 는 zip 을 받아 첫 .mst 파일을 파싱한다. 인증이 필요 없다.
func (c *Client) DownloadMaster(ctx context.Context, market string) ([]MasterRow, error) {
	if _, ok := masterLayouts[market]; !ok {
		return nil, fmt.Errorf("kis: unknown market %q", market)
	}
	base := c.MasterBaseURL
	if base == "" {
		base = DefaultMasterBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+MasterURL(market), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kis: master download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kis: master download: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("kis: master zip: %w", err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".mst") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return ParseMaster(rc, market)
	}
	return nil, fmt.Errorf("kis: master zip has no .mst file")
}
```

`internal/kis/client.go`의 `Client` 구조체에 필드 추가 (`HTTP *http.Client` 아래):

```go
	MasterBaseURL  string // 비어 있으면 DefaultMasterBaseURL
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/kis/ -v 2>&1 | tail -20`
Expected: 전부 PASS (16개)

- [ ] **Step 6: 커밋 요청**

```bash
git add go.mod go.sum internal/kis
git commit -m "feat(kis): 종목 마스터 파일 다운로드와 파싱"
```

---

### Task 2: 유니버스 갱신 오케스트레이션

**Files:**
- Create: `internal/app/universe.go`
- Test: `internal/app/universe_test.go`

**Interfaces:**
- Consumes: `config.Config`, `data.Store`, `kis.MasterRow`, `kis.KnownFlags`.
- Produces: `app.MasterSource` 인터페이스, `app.RunUniverse(ctx, cfg *config.Config, store data.Store, src MasterSource) (UniverseResult, error)`.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/app/universe_test.go`:

```go
package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
	"github.com/gong-yeongbin/my-trading/internal/tui"
)

type fakeMaster struct {
	rows map[string][]kis.MasterRow
	err  error
}

func (f fakeMaster) DownloadMaster(_ context.Context, market string) ([]kis.MasterRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[market], nil
}

func testConfig() *config.Config {
	return &config.Config{
		Universe: config.UniverseConfig{Markets: []string{"kospi", "kosdaq"}, GroupCodes: []string{"ST"},
			ExcludeFlags: []string{"거래정지", "관리종목", "SPAC"}},
	}
}

func openStore(t *testing.T) *data.SQLiteStore {
	t.Helper()
	s, err := data.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRunUniverseFilters(t *testing.T) {
	src := fakeMaster{rows: map[string][]kis.MasterRow{
		"kospi": {
			{Code: "005930", Name: "삼성전자", Market: "kospi", GroupCode: "ST", WarningCode: "00", OverheatCode: "0"},
			{Code: "069500", Name: "KODEX 200", Market: "kospi", GroupCode: "EF", WarningCode: "00", OverheatCode: "0"},
			{Code: "000040", Name: "KR모터스", Market: "kospi", GroupCode: "ST", Managed: true, WarningCode: "00", OverheatCode: "0"},
		},
		"kosdaq": {
			{Code: "247540", Name: "에코프로비엠", Market: "kosdaq", GroupCode: "ST", WarningCode: "02", OverheatCode: "0"},
			{Code: "0004Y0", Name: "디비금융제14호스팩", Market: "kosdaq", GroupCode: "ST", Spac: true, WarningCode: "00", OverheatCode: "0"},
		},
	}}
	store := openStore(t)
	res, err := RunUniverse(context.Background(), testConfig(), store, src)
	if err != nil {
		t.Fatal(err)
	}
	syms, _ := store.ListSymbols(context.Background())
	if len(syms) != 2 || syms[0].Code != "005930" || syms[1].Code != "247540" || syms[1].Market != "kosdaq" {
		t.Errorf("symbols = %+v", syms)
	}
	if res.Downloaded != 5 || res.Kept != 2 || res.ByMarket["kospi"] != 1 || res.ByMarket["kosdaq"] != 1 {
		t.Errorf("result = %+v", res)
	}
}

func TestRunUniverseRejectsUnknownFlagBeforeDownload(t *testing.T) {
	cfg := testConfig()
	cfg.Universe.ExcludeFlags = []string{"없는플래그"}
	src := fakeMaster{err: errors.New("must not be called")}
	if _, err := RunUniverse(context.Background(), cfg, openStore(t), src); err == nil || err.Error() == "must not be called" {
		t.Errorf("expected flag validation error before download, got %v", err)
	}
}

func TestRunUniversePropagatesDownloadError(t *testing.T) {
	src := fakeMaster{err: errors.New("network down")}
	if _, err := RunUniverse(context.Background(), testConfig(), openStore(t), src); err == nil {
		t.Error("expected error")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/app/ 2>&1 | head -5`
Expected: 컴파일 실패

- [ ] **Step 3: 구현**

`internal/app/universe.go`:

```go
// Package app 은 서브커맨드와 TUI 가 공유하는 실행 로직이다.
package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
	"github.com/gong-yeongbin/my-trading/internal/tui"
)

type MasterSource interface {
	DownloadMaster(ctx context.Context, market string) ([]kis.MasterRow, error)
}

type UniverseResult struct {
	Downloaded int
	Kept       int
	ByMarket   map[string]int
}

// RunUniverse 는 마스터 파일을 받아 그룹코드·제외 플래그로 거른 종목을 symbols 테이블에 저장한다.
// 기존 종목은 갱신되고, 마스터에서 사라진 종목은 삭제하지 않는다 (봉 이력 보존).
func RunUniverse(ctx context.Context, cfg *config.Config, store data.Store, src MasterSource) (UniverseResult, error) {
	for _, flag := range cfg.Universe.ExcludeFlags {
		if _, err := (kis.MasterRow{}).FlagOn(flag); err != nil {
			return UniverseResult{}, fmt.Errorf("universe.exclude_flags: %w", err)
		}
	}
	res := UniverseResult{ByMarket: map[string]int{}}
	var keep []data.Symbol
	for _, market := range cfg.Universe.Markets {
		rows, err := src.DownloadMaster(ctx, market)
		if err != nil {
			return UniverseResult{}, fmt.Errorf("universe: %s: %w", market, err)
		}
		res.Downloaded += len(rows)
		for _, r := range rows {
			if !slices.Contains(cfg.Universe.GroupCodes, r.GroupCode) {
				continue
			}
			if excluded(r, cfg.Universe.ExcludeFlags) {
				continue
			}
			keep = append(keep, data.Symbol{Code: r.Code, Name: r.Name, Market: r.Market})
			res.ByMarket[market]++
		}
	}
	if err := store.UpsertSymbols(ctx, keep); err != nil {
		return UniverseResult{}, fmt.Errorf("universe: save: %w", err)
	}
	res.Kept = len(keep)
	return res, nil
}

func excluded(r kis.MasterRow, flags []string) bool {
	for _, flag := range flags {
		if on, _ := r.FlagOn(flag); on {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/app/ -v 2>&1 | tail -8`
Expected: 3개 PASS

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/app
git commit -m "feat(app): 유니버스 갱신"
```

---

### Task 3: 일봉 증분 수집 오케스트레이션

**Files:**
- Create: `internal/app/fetch.go`
- Test: `internal/app/fetch_test.go`

**Interfaces:**
- Consumes: `data.Store`, 5단계의 `DailyBars`, `IndexBars` 시그니처.
- Produces: `app.BarSource` 인터페이스, `app.FetchOptions{From time.Time; Today time.Time}`, `app.FetchProgress{Done, Total int; Code string; Err error}`, `app.FetchResult{Symbols, Bars int; Failures []FetchFailure}`, `app.RunFetch(ctx, cfg, store, src BarSource, opts FetchOptions, progress func(FetchProgress)) (FetchResult, error)`.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/app/fetch_test.go`:

```go
package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

type call struct {
	kind, key string
	from, to  time.Time
}

type fakeBars struct {
	calls    []call
	failCode string
	indexErr error
}

func (f *fakeBars) DailyBars(_ context.Context, code string, from, to time.Time) ([]data.Bar, error) {
	f.calls = append(f.calls, call{"bar", code, from, to})
	if code == f.failCode {
		return nil, errors.New("boom")
	}
	var out []data.Bar
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, data.Bar{Date: d, Open: 1, High: 2, Low: 1, Close: 2, Volume: 3})
	}
	return out, nil
}

func (f *fakeBars) IndexBars(_ context.Context, market string, from, to time.Time) ([]data.IndexBar, error) {
	f.calls = append(f.calls, call{"index", market, from, to})
	if f.indexErr != nil {
		return nil, f.indexErr
	}
	var out []data.IndexBar
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, data.IndexBar{Date: d, Open: 1, High: 2, Low: 1, Close: 2})
	}
	return out, nil
}

func fetchConfig() *config.Config {
	return &config.Config{
		Universe: config.UniverseConfig{Markets: []string{"kospi"}},
		Fetch:    config.FetchConfig{StartDate: "2024-09-01"},
	}
}

func TestRunFetchInitialAndIncremental(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{"005930", "삼성전자", "kospi"}, {"000660", "SK하이닉스", "kospi"}})
	src := &fakeBars{}
	today := data.Date(2024, 9, 10)
	var progress []FetchProgress
	res, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: today}, func(p FetchProgress) { progress = append(progress, p) })
	if err != nil {
		t.Fatal(err)
	}
	// 지수 먼저, 그 다음 종목 순서
	if len(src.calls) != 3 || src.calls[0].kind != "index" || !src.calls[0].from.Equal(data.Date(2024, 9, 1)) || !src.calls[0].to.Equal(today) {
		t.Fatalf("calls = %+v", src.calls)
	}
	if src.calls[1].key != "000660" || src.calls[2].key != "005930" || !src.calls[1].from.Equal(data.Date(2024, 9, 1)) {
		t.Errorf("symbol calls = %+v", src.calls[1:])
	}
	if res.Symbols != 2 || res.Bars != 20 || len(res.Failures) != 0 {
		t.Errorf("result = %+v", res)
	}
	if len(progress) != 2 || progress[1].Done != 2 || progress[1].Total != 2 {
		t.Errorf("progress = %+v", progress)
	}
	if last, ok, _ := store.LastIndexBarDate(ctx, "kospi"); !ok || !last.Equal(today) {
		t.Errorf("index not stored: %v %v", last, ok)
	}

	// 증분: 마지막 저장일 다음 날부터
	src.calls = nil
	today2 := data.Date(2024, 9, 12)
	if _, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: today2}, nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range src.calls {
		if !c.from.Equal(data.Date(2024, 9, 11)) || !c.to.Equal(today2) {
			t.Errorf("incremental range wrong: %+v", c)
		}
	}
	// 이미 오늘까지 있으면 호출하지 않는다
	src.calls = nil
	if _, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: today2}, nil); err != nil {
		t.Fatal(err)
	}
	if len(src.calls) != 0 {
		t.Errorf("expected no calls when up to date, got %+v", src.calls)
	}
}

func TestRunFetchFromOverridesStartOnlyWhenEmpty(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{"005930", "삼성전자", "kospi"}})
	store.UpsertBars(ctx, "005930", []data.Bar{{Date: data.Date(2024, 9, 5), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1}})
	src := &fakeBars{}
	_, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{From: data.Date(2024, 8, 1), Today: data.Date(2024, 9, 10)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !src.calls[0].from.Equal(data.Date(2024, 8, 1)) { // 지수는 비어 있으므로 --from
		t.Errorf("index from = %v", src.calls[0].from)
	}
	if !src.calls[1].from.Equal(data.Date(2024, 9, 6)) { // 종목은 저장분 다음 날
		t.Errorf("symbol from = %v", src.calls[1].from)
	}
}

func TestRunFetchContinuesOnSymbolFailure(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{"000660", "SK하이닉스", "kospi"}, {"005930", "삼성전자", "kospi"}})
	src := &fakeBars{failCode: "000660"}
	res, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: data.Date(2024, 9, 2)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failures) != 1 || res.Failures[0].Code != "000660" || res.Symbols != 1 {
		t.Errorf("result = %+v", res)
	}
	if _, ok, _ := store.LastBarDate(ctx, "005930"); !ok {
		t.Error("healthy symbol should still be stored")
	}
}

func TestRunFetchStopsOnIndexFailure(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	store.UpsertSymbols(ctx, []data.Symbol{{"005930", "삼성전자", "kospi"}})
	src := &fakeBars{indexErr: errors.New("index down")}
	if _, err := RunFetch(ctx, fetchConfig(), store, src, FetchOptions{Today: data.Date(2024, 9, 2)}, nil); err == nil {
		t.Fatal("expected error")
	}
	if len(src.calls) != 1 {
		t.Errorf("symbols must not be fetched after index failure: %+v", src.calls)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/app/ -run Fetch 2>&1 | head -5`
Expected: 컴파일 실패

- [ ] **Step 3: 구현**

`internal/app/fetch.go`:

```go
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
)

type BarSource interface {
	DailyBars(ctx context.Context, code string, from, to time.Time) ([]data.Bar, error)
	IndexBars(ctx context.Context, market string, from, to time.Time) ([]data.IndexBar, error)
}

type FetchOptions struct {
	From  time.Time // 저장된 봉이 없을 때의 시작일. 비어 있으면 fetch.start_date
	Today time.Time // 비어 있으면 KST 오늘
}

type FetchProgress struct {
	Done  int
	Total int
	Code  string
	Err   error
}

type FetchFailure struct {
	Code string
	Err  error
}

type FetchResult struct {
	Symbols  int // 성공한 종목 수
	Bars     int // 저장한 봉 수
	Failures []FetchFailure
}

// RunFetch 는 지수 일봉을 먼저(실패 시 즉시 중단), 이어서 종목 일봉을 증분 수집한다.
// 종목 하나가 실패해도 계속 진행하고 Failures 에 기록한다.
func RunFetch(ctx context.Context, cfg *config.Config, store data.Store, src BarSource, opts FetchOptions, progress func(FetchProgress)) (FetchResult, error) {
	today := opts.Today
	if today.IsZero() {
		now := time.Now().In(data.KST)
		today = data.Date(now.Year(), now.Month(), now.Day())
	}
	start := opts.From
	if start.IsZero() {
		var err error
		if start, err = data.ParseDate(cfg.Fetch.StartDate); err != nil {
			return FetchResult{}, fmt.Errorf("fetch: start_date: %w", err)
		}
	}

	for _, market := range cfg.Universe.Markets {
		last, ok, err := store.LastIndexBarDate(ctx, market)
		if err != nil {
			return FetchResult{}, err
		}
		from := nextFrom(start, last, ok)
		if from.After(today) {
			continue
		}
		bars, err := src.IndexBars(ctx, market, from, today)
		if err != nil {
			return FetchResult{}, fmt.Errorf("fetch: index %s: %w", market, err)
		}
		if err := store.UpsertIndexBars(ctx, market, bars); err != nil {
			return FetchResult{}, fmt.Errorf("fetch: save index %s: %w", market, err)
		}
	}

	syms, err := store.ListSymbols(ctx)
	if err != nil {
		return FetchResult{}, err
	}
	var res FetchResult
	for i, sym := range syms {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		err := fetchSymbol(ctx, store, src, sym.Code, start, today, &res)
		if err != nil {
			res.Failures = append(res.Failures, FetchFailure{Code: sym.Code, Err: err})
		} else {
			res.Symbols++
		}
		if progress != nil {
			progress(FetchProgress{Done: i + 1, Total: len(syms), Code: sym.Code, Err: err})
		}
	}
	return res, nil
}

func fetchSymbol(ctx context.Context, store data.Store, src BarSource, code string, start, today time.Time, res *FetchResult) error {
	last, ok, err := store.LastBarDate(ctx, code)
	if err != nil {
		return err
	}
	from := nextFrom(start, last, ok)
	if from.After(today) {
		return nil
	}
	bars, err := src.DailyBars(ctx, code, from, today)
	if err != nil {
		return err
	}
	if len(bars) == 0 {
		return nil
	}
	if err := store.UpsertBars(ctx, code, bars); err != nil {
		return err
	}
	res.Bars += len(bars)
	return nil
}

// nextFrom 은 저장된 마지막 날짜가 있으면 그 다음 날, 없으면 start 를 돌려준다.
func nextFrom(start, last time.Time, hasLast bool) time.Time {
	if hasLast {
		return last.AddDate(0, 0, 1)
	}
	return start
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/app/ -v 2>&1 | tail -12`
Expected: 7개 PASS

- [ ] **Step 5: 커밋 요청**

```bash
git add internal/app
git commit -m "feat(app): 지수·종목 일봉 증분 수집"
```

---

### Task 4: CLI 서브커맨드 추가와 실서버 수동 확인

**Files:**
- Modify: `cmd/trader/main.go` (1단계에서 작성됨. 인자 없으면 TUI 실행하는 분기는 유지하고 `universe`, `fetch` 케이스와 `runUniverse`, `runFetch`, `newClient`를 추가한다)

**Interfaces:**
- Consumes: `config.LoadDotEnv`, `config.Load`, `(*Config).RequireAppKey`, `data.Open`, `kis.New`, `app.RunUniverse`, `app.RunFetch`.
- Produces: `trader universe`, `trader fetch [--from YYYY-MM-DD]`. 인자 없는 실행(TUI)은 1단계 그대로.

- [ ] **Step 1: main.go에 서브커맨드 추가**

아래는 완성 형태다. 1단계의 `run` 골격(`.env`·config 로딩, 인자 없으면 `tui.Run`)은 유지하고 `switch`에 케이스를 더한다.

```go
// trader 는 종가 베팅 운영 도구의 CLI 진입점이다.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gong-yeongbin/my-trading/internal/app"
	"github.com/gong-yeongbin/my-trading/internal/config"
	"github.com/gong-yeongbin/my-trading/internal/data"
	"github.com/gong-yeongbin/my-trading/internal/kis"
	"github.com/gong-yeongbin/my-trading/internal/tui"
)

const usage = `사용법:
  trader                 TUI
  trader universe        마스터 파일로 종목 목록 갱신
  trader fetch [--from YYYY-MM-DD]   지수·종목 일봉 증분 수집
  trader watch           (7단계 계획에서 구현)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf(".env: %w", err)
	}
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) == 0 {
		return tui.Run(ctx, cfg)
	}
	switch args[0] {
	case "universe":
		return runUniverse(ctx, cfg)
	case "fetch":
		return runFetch(ctx, cfg, args[1:])
	case "watch":
		return fmt.Errorf("%s 는 아직 구현되지 않았습니다 (7단계)", args[0])
	default:
		fmt.Print(usage)
		return fmt.Errorf("알 수 없는 명령 %q", args[0])
	}
}

func newClient(cfg *config.Config) (*kis.Client, error) {
	if err := cfg.RequireAppKey(); err != nil {
		return nil, err
	}
	return kis.New(cfg.KIS.BaseURL(), cfg.KIS.AppKey, cfg.KIS.AppSecret, cfg.KIS.TokenCache, cfg.KIS.EffectiveRPS()), nil
}

func runUniverse(ctx context.Context, cfg *config.Config) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	store, err := data.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	res, err := app.RunUniverse(ctx, cfg, store, client)
	if err != nil {
		return err
	}
	fmt.Printf("마스터 %d 종목 중 %d 종목 저장", res.Downloaded, res.Kept)
	for _, m := range cfg.Universe.Markets {
		fmt.Printf("  %s %d", m, res.ByMarket[m])
	}
	fmt.Println()
	return nil
}

func runFetch(ctx context.Context, cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	fromStr := fs.String("from", "", "저장된 봉이 없을 때의 시작일 (YYYY-MM-DD)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var opts app.FetchOptions
	if *fromStr != "" {
		from, err := data.ParseDate(*fromStr)
		if err != nil {
			return fmt.Errorf("--from: %w", err)
		}
		opts.From = from
	}
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	store, err := data.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	started := time.Now()
	fmt.Fprintf(os.Stderr, "[%s] 지수 일봉 수집 중...\n", cfg.KIS.Env)
	res, err := app.RunFetch(ctx, cfg, store, client, opts, func(p app.FetchProgress) {
		if p.Err != nil {
			fmt.Fprintf(os.Stderr, "\r[%d/%d] %s 실패: %v\n", p.Done, p.Total, p.Code, p.Err)
			return
		}
		fmt.Fprintf(os.Stderr, "\r[%d/%d] %s", p.Done, p.Total, p.Code)
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	fmt.Printf("종목 %d 성공, 봉 %d 저장, 실패 %d, 소요 %s\n", res.Symbols, res.Bars, len(res.Failures), time.Since(started).Round(time.Second))
	for _, f := range res.Failures {
		fmt.Printf("  실패 %s: %v\n", f.Code, f.Err)
	}
	return nil
}
```

- [ ] **Step 2: 빌드와 전체 테스트**

Run: `gofmt -l . ; go vet ./... && go build ./... && go test ./... 2>&1 | tail -8`
Expected: gofmt 출력 없음, vet 통과, 모든 패키지 `ok`

- [ ] **Step 3: 잘못된 명령 확인**

Run: `go run ./cmd/trader nope; echo "exit=$?"`
Expected: 사용법 출력 후 exit=1

- [ ] **Step 4: 실서버 수동 확인 (사용자 협업, 모의투자 앱키)**

Claude는 `.env`를 읽을 수 없으므로 프로그램 실행 결과로만 확인한다. `.env`에 `KIS_APP_KEY`, `KIS_APP_SECRET`이 있어야 한다.

1. 유니버스: `go run ./cmd/trader universe`
   Expected: `마스터 4403 종목 중 약 2500 종목 저장  kospi 약 800  kosdaq 약 1500` 수준의 출력 (2026-09 기준 코스피 ST 915, 코스닥 ST 1803에서 플래그 제외분을 뺀 수). 실패하면 오류 메시지를 사용자에게 그대로 보여준다.
2. 짧은 기간 수집으로 API 동작 확인: `go run ./cmd/trader fetch --from 2026-09-01`
   Expected: 지수 2개 시장 저장 후 종목 진행률이 올라가고, 모의 앱키(초당 1.5건)면 2,500종목에 약 30분. 확인만 목적이면 `Ctrl+C`로 중단해도 된다 (종목 단위 트랜잭션이라 중단 시점까지는 저장됨).
3. 저장 확인:
   ```bash
   sqlite3 data/market.db "select market, count(*), min(date), max(date) from index_bars group by market; select count(distinct code), count(*) from bars;"
   ```
   Expected: 지수 2행에 9월 영업일 수만큼, bars 에 진행된 종목 수만큼.
4. 증분 확인: 같은 `fetch`를 다시 실행하면 이미 오늘까지 받은 종목은 호출 없이 지나가야 한다 (진행률이 빠르게 올라감).
5. 이후 본 수집은 사용자가 `config.yaml`의 `fetch.start_date`(기본 2021-01-01)로 `go run ./cmd/trader fetch`를 밤에 실행한다. **주의: 2번의 `--from` 수집으로 마지막 저장일이 생기면 본 수집은 증분이 되어 2021~2026-08 구간을 절대 받지 않는다.** 본 수집 전에 `sqlite3 data/market.db "delete from bars; delete from index_bars;"` 로 봉을 비우거나, 2번을 건너뛰고 처음부터 본 수집을 돌린 뒤 Ctrl+C·재실행으로 이어받는다. 소요: 2,500종목 × 15청크 ≈ 37,500건 ÷ 1.5/s ≈ 6~7시간 (모의 앱키). 장 마감(15:30) 후에 돌려야 당일 봉이 확정값으로 저장된다. 이 단계는 계획 완료 조건이 아니다.

- [ ] **Step 5: 커밋 요청**

```bash
git add cmd/trader
git commit -m "feat(cli): universe, fetch 서브커맨드"
```

---


## 완료 기준

- `go test ./...` 전부 통과, `go vet ./...` 통과, `gofmt -l .` 비어 있음
- 모의 앱키로 `trader universe`와 짧은 기간 `trader fetch`가 실서버에서 성공하고 SQLite에 지수·종목 봉이 들어감
- 다음 단계(7단계 전략 조건 함수·관심 종목 선별·`watch`)는 `data.Store`, `data.Bar`, `data.IndexBar`, `config.StrategyConfig`를 그대로 소비한다
