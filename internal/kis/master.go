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
var KnownFlags = []string{"거래정지", "정리매매", "관리종목", "시장경고", "단기과열", "이상급등", "SPAC", "우선주"}

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
	Preferred    bool // 우선주 (마스터 우선주 구분 0=보통주, 1 구형, 2 신형, 9 기타)
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
	case "우선주":
		return r.Preferred, nil
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
		Preferred:    strings.TrimSpace(fields["우선주"]) != "0",
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
