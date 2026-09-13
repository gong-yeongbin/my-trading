# 국내 주식 종가 베팅 백테스트 시스템 설계

작성일: 2026-09-13 (2차 개정: 진입·청산 규칙, 지수 필터, 유니버스 플래그 필터 반영. 3차 개정: 관심 종목 선별과 TUI 추가)

## 1. 목표와 범위

개인 프로그램 매매를 위한 첫 단계로, 국내 주식 일봉 기반 **종가 베팅 전략의 백테스트 환경**을 Go로 만든다.

이번 범위에 포함:

- 한국투자증권(한투) Open API로 종목 일봉과 지수 일봉 수집, 로컬 SQLite 저장
- 한투 종목 마스터 파일로 유니버스 구성 (관리종목·거래정지 등 플래그 제외)
- 종가 베팅 전략 1개 (시장 필터, 마감 강도, 신고가, 조건부 보유 연장)
- 이벤트 기반 일봉 백테스트 엔진 (체결, 수수료, 거래세, 슬리피지)
- 성과 리포트 (요약 지표, 거래 내역 CSV, 자산 곡선 CSV)
- 관심 종목 선별: 전일 데이터로 "오늘 진입 조건을 통과하려면 필요한 종가·거래량" 문턱값 계산
- 대화형 TUI: 대시보드, 관심 종목, 종목 상세, 전략, 지수, 수집, 백테스트 실행, 결과 화면

이번 범위에서 제외 (나중 단계):

- 모의투자/실전 주문
- 실시간 시세 (웹소켓, 틱)
- 장중 손절/익절
- 투자자별 수급(외국인·기관 순매수) 조건
- 공매도, 레버리지, 분할 매수
- 차트 출력
- 상장폐지 종목, 과거 시점의 관리종목 지정 이력 반영 (생존 편향 보정)

## 2. 결정 사항 요약

| 항목 | 결정 | 이유 |
|---|---|---|
| 시장 | 국내 주식, 일봉 | 원화 단일, 나중에 실전도 같은 API |
| 언어 | Go 단일 | 실전 봇 확장 시 동시성·배포 유리. 백테스트 성능은 충분 |
| 데이터 소스 | 한투 Open API | 공식 지원, 수정주가 제공, 실전까지 하나의 API |
| 저장소 | SQLite (`modernc.org/sqlite`, CGO 불필요) | 백테스트가 네트워크 없이 로컬만 읽도록 |
| 유니버스 | 한투 마스터 파일, 코스피+코스닥 보통주 전체, 플래그 종목 제외 | 종가 베팅은 스캔형이라 넓은 유니버스 필요 |
| 전략 스타일 | 종가 베팅 (종가 매수, 익일 시가 매도, 조건부 연장) | 사용자 선택 |
| 시장 필터 | 종목이 속한 시장의 지수(코스피 0001 / 코스닥 1001)가 20일선 위일 때만 진입 | 하락장 갭 하락 회피 |
| 체결 모델 | 매수는 당일 종가+슬리피지, 매도는 익일 시가 또는 당일 종가 | 일봉만으로 재현 가능 |
| 화면 | 대화형 TUI (bubbletea) + 비대화형 서브커맨드 병행 | 매일 보는 운영 대시보드 용도. 스크립트 연동은 서브커맨드로 |

## 3. 디렉터리 구조

```
my-trading/
  cmd/trader/main.go          CLI 진입점: universe, fetch, backtest
  internal/config/            config.yaml 로딩, 환경변수(앱키) 읽기
  internal/kis/               한투 API 클라이언트: 토큰, 종목/지수 일봉, 호출 제한, 마스터 파일
  internal/data/              Bar·IndexBar 타입, Store 인터페이스, SQLite 구현
  internal/strategy/          Strategy 인터페이스, 종가 베팅 전략
  internal/backtest/          엔진, 포트폴리오, 체결/비용 모델
  internal/report/            지표 계산, 터미널 출력, CSV 출력
  internal/screener/          관심 종목 선별: 전일 봉으로 진입 문턱값 계산
  internal/tui/               bubbletea 화면들. 다른 패키지를 호출만 하고 로직은 갖지 않음
  config.yaml                 종목 필터, 전략 파라미터, 비용, 사이징
  data/market.db              SQLite (git 제외)
  docs/superpowers/specs/     설계 문서
```

외부 의존성은 SQLite 드라이버, YAML 파서(`gopkg.in/yaml.v3`), TUI용 `charmbracelet/bubbletea`·`bubbles`·`lipgloss`, ASCII 차트용 `guptarohit/asciigraph`로 제한한다. `data/`, `out/`, `.env`는 `.gitignore`에 넣는다.

## 4. CLI

```
trader                     인자 없이 실행하면 TUI 시작
trader universe            마스터 파일 다운로드 → 필터 적용 → symbols 테이블 갱신
trader fetch [--from DATE] symbols 테이블의 종목 일봉 + 코스피/코스닥 지수 일봉을 증분 수집
trader backtest [--from DATE] [--to DATE]   SQLite만 읽어 백테스트 실행, 리포트 출력
trader watch               전일 데이터 기준 관심 종목을 표로 출력
```

`fetch`와 `universe`만 네트워크를 쓴다. `backtest`, `watch`, TUI의 조회 화면은 네트워크를 쓰지 않는다. TUI 안의 수집·백테스트 실행은 서브커맨드와 같은 함수를 호출한다.

`fetch`는 종목(및 지수)마다 마지막 저장일 다음 날부터 오늘까지 받는다. 저장된 봉이 없으면 `--from`(없으면 `fetch.start_date`)부터 받는다.

## 5. 설정 (config.yaml)

```yaml
kis:
  base_url: https://openapi.koreainvestment.com:9443
  # app_key, app_secret 은 환경변수 KIS_APP_KEY, KIS_APP_SECRET 에서 읽음
  token_cache: data/token.json
  requests_per_second: 15      # 실전 한도(커뮤니티 알려진 값 20/s, 포털에서 확인) 아래로 여유

universe:
  markets: [kospi, kosdaq]
  group_codes: [ST]            # 보통주만
  exclude_flags: [거래정지, 정리매매, 관리종목, 시장경고, 단기과열, 이상급등, SPAC]

fetch:
  start_date: "2021-01-01"     # 최초 수집 시작일

strategy:
  # 시장 필터
  index_ma_days: 20            # 지수 종가 > 지수 20일 이동평균
  # 유동성
  min_turnover: 10000000000    # 당일 거래대금 하한 (원, 100억)
  turnover_ma_days: 20
  turnover_ratio_min: 3.0      # 당일 거래대금 >= 20일 평균 거래대금 × 3
  # 마감 강도
  close_to_high_min: 0.99      # 종가 >= 당일 고가 × 0.99
  # 상승률
  change_min: 0.03
  change_max: 0.20             # 상한가 근처 제외
  # 추세
  ma_short_days: 20            # 종가 > 20일선 > 60일선
  ma_long_days: 60
  new_high_days: 20            # 종가 >= 직전 20일 고가 최댓값
  # 청산
  gap_hold_min: 0.02           # 익일 시가 갭 >= +2% 면 그날 종가까지 보유

sizing:
  initial_cash: 10000000
  position_pct: 0.10           # 종목당 총자산의 10%
  max_entries_per_day: 5

cost:
  commission_rate: 0.00015     # 매수·매도 각각
  tax_rate: 0.0015             # 매도 시 거래세
  close_slippage: 0.002        # 종가 체결 시 불리하게 적용
```

## 6. 데이터 계층

### 6.1 타입

```go
type Bar struct {
    Date   time.Time // 영업일, KST 자정
    Open   int64     // 원 단위 정수
    High   int64
    Low    int64
    Close  int64
    Volume int64
}

type IndexBar struct {
    Date  time.Time
    Open  float64   // 지수는 소수점이 있으므로 float64
    High  float64
    Low   float64
    Close float64
}
```

종목 가격은 원 단위 정수로 다룬다. 국내 주식은 소수점 가격이 없고, 부동소수 오차를 피한다. 수정주가로 받으므로 별도 보정은 없다. 거래대금은 `Close × Volume`으로 근사한다 (API의 누적 거래대금 필드는 쓰지 않는다. 수정주가 적용 시 일관성이 불명확하기 때문).

### 6.2 SQLite 스키마

```sql
CREATE TABLE symbols (
  code   TEXT PRIMARY KEY,   -- 6자리 단축코드
  name   TEXT NOT NULL,
  market TEXT NOT NULL       -- kospi | kosdaq
);

CREATE TABLE bars (
  code   TEXT NOT NULL,
  date   TEXT NOT NULL,      -- YYYY-MM-DD
  open   INTEGER NOT NULL,
  high   INTEGER NOT NULL,
  low    INTEGER NOT NULL,
  close  INTEGER NOT NULL,
  volume INTEGER NOT NULL,
  PRIMARY KEY (code, date)
);

CREATE TABLE index_bars (
  market TEXT NOT NULL,      -- kospi | kosdaq
  date   TEXT NOT NULL,
  open   REAL NOT NULL,
  high   REAL NOT NULL,
  low    REAL NOT NULL,
  close  REAL NOT NULL,
  PRIMARY KEY (market, date)
);
```

### 6.3 Store 인터페이스

```go
type Store interface {
    UpsertSymbols(ctx context.Context, syms []Symbol) error
    ListSymbols(ctx context.Context) ([]Symbol, error)
    LastBarDate(ctx context.Context, code string) (time.Time, bool, error)
    UpsertBars(ctx context.Context, code string, bars []Bar) error
    LoadBars(ctx context.Context, code string, from, to time.Time) ([]Bar, error)
    LastIndexBarDate(ctx context.Context, market string) (time.Time, bool, error)
    UpsertIndexBars(ctx context.Context, market string, bars []IndexBar) error
    LoadIndexBars(ctx context.Context, market string, from, to time.Time) ([]IndexBar, error)
}
```

## 7. 한투 API 클라이언트 (internal/kis)

공식 GitHub 예제(`koreainvestment/open-trading-api`)에서 확인한 사양을 따른다.

### 7.1 토큰

- `POST {base_url}/oauth2/tokenP`, body: `grant_type=client_credentials`, `appkey`, `appsecret`
- 응답의 `access_token`, `access_token_token_expired`(만료 일시)를 `token_cache` 파일에 저장
- 만료 전이면 파일의 토큰을 재사용한다. 재발급은 분당 1회 제한이므로 매 실행마다 발급하면 안 된다.
- API 호출이 401을 돌려주면 토큰을 한 번 재발급하고 같은 요청을 한 번 재시도한다.

### 7.2 종목 일봉 조회

- `GET /uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice`
- 헤더: `authorization: Bearer {token}`, `appkey`, `appsecret`, `tr_id: FHKST03010100`
- 파라미터: `FID_COND_MRKT_DIV_CODE=J`, `FID_INPUT_ISCD={code}`, `FID_INPUT_DATE_1={시작 YYYYMMDD}`, `FID_INPUT_DATE_2={종료 YYYYMMDD}`, `FID_PERIOD_DIV_CODE=D`, `FID_ORG_ADJ_PRC=0`(수정주가)
- 응답 `output2` 배열의 필드: `stck_bsop_date`, `stck_oprc`, `stck_hgpr`, `stck_lwpr`, `stck_clpr`, `acml_vol`
- **호출당 최대 100건.** 요청 기간을 종료일에서 거꾸로 140일(달력일 기준, 영업일 약 100일) 단위로 잘라 반복 호출하고, 응답이 비거나 시작일 이전에 도달하면 멈춘다. 응답에 빈 문자열 가격이 섞인 행(거래정지 등)은 건너뛴다.

### 7.3 지수 일봉 조회

- `GET /uapi/domestic-stock/v1/quotations/inquire-daily-indexchartprice`
- 헤더: 종목 일봉과 동일, `tr_id: FHKUP03500100`
- 파라미터: `FID_COND_MRKT_DIV_CODE=U`, `FID_INPUT_ISCD=0001`(코스피 종합) 또는 `1001`(코스닥 종합), `FID_INPUT_DATE_1`, `FID_INPUT_DATE_2`, `FID_PERIOD_DIV_CODE=D`
- 응답 `output2` 배열의 필드: `stck_bsop_date`, `bstp_nmix_oprc`, `bstp_nmix_hgpr`, `bstp_nmix_lwpr`, `bstp_nmix_prpr`(종가)
- 종목 일봉과 같은 방식으로 기간을 잘라 반복 호출한다.

### 7.4 호출 제한

- 토큰 버킷 방식 rate limiter로 `requests_per_second`를 지킨다.
- 초당 제한 초과 에러(`EGW00201`)를 받으면 1초 대기 후 같은 요청을 재시도한다 (최대 3회).

### 7.5 마스터 파일

- `https://new.real.download.dws.co.kr/common/master/kospi_code.mst.zip`, `kosdaq_code.mst.zip` 다운로드 (인증 불필요)
- 압축 해제 후 CP949 → UTF-8 변환, 한 줄이 한 종목
- 앞부분: 단축코드(9자, 앞 6자 사용), 표준코드(12자), 한글명(가변). 뒷부분 228자(코스피) 고정폭 필드. 코스닥은 뒷부분 폭이 다르므로 공식 예제(`stocks_info/kis_kospi_code_mst.py`, `kis_kosdaq_code_mst.py`)의 필드 폭과 컬럼명을 그대로 옮긴다.
- 사용하는 필드: `그룹코드`(ST=보통주), `거래정지`, `정리매매`, `관리종목`, `시장경고`(0 아니면 투자주의/경고/위험), `단기과열`, `이상급등`, `SPAC`. 코스닥 파일의 대응 컬럼명은 예제 코드를 따른다.
- `universe.group_codes`에 해당하고 `universe.exclude_flags`에 해당하는 플래그가 모두 꺼진 종목만 `symbols` 테이블에 저장한다.
- **한계:** 플래그는 현재 시점 값이다. 과거에 관리종목이었다가 해제된 종목은 포함되고, 현재 관리종목인 종목은 과거 정상 기간까지 제외된다. 1차에서는 이 근사를 감수하고 리포트에 명시한다.

## 8. 전략 인터페이스와 종가 베팅 전략

### 8.1 인터페이스

```go
type Action int
const (
    Hold Action = iota
    Buy
    Sell
)

type Position struct {
    EntryDate  time.Time
    EntryPrice int64
    Qty        int64
    Extended   bool   // 보유 연장했는지
}

// 종목이 속한 시장의 지수 일봉. 종목 bars 와 같은 규칙으로 당일까지만 담긴다.
type Market struct {
    IndexBars []IndexBar
}

type Strategy interface {
    // 장 마감 시점. bars 는 해당 종목의 당일까지 일봉(과거→현재), 마지막이 당일.
    // pos 가 nil 이면 미보유: Buy 또는 Hold 를 반환.
    // pos 가 있으면 보유 중: Sell(당일 종가 매도) 또는 Hold 를 반환.
    OnClose(mkt Market, bars []Bar, pos *Position) Action

    // 익일 시가 시점. 보유 종목에 대해서만 호출. bars 는 전날까지, open 은 당일 시가.
    // Sell(시가 매도) 또는 Hold(보유 연장) 를 반환.
    OnOpen(bars []Bar, open int64, pos *Position) Action

    // 같은 날 Buy 후보가 max_entries_per_day 를 넘을 때 우선순위. 클수록 우선.
    Score(bars []Bar) float64
}
```

전략은 미래 봉을 절대 받지 않는다. 엔진이 `bars[:i+1]`, `IndexBars[:j+1]`만 넘긴다.

### 8.2 종가 베팅 전략 규칙

**진입 (OnClose, 미보유):** 아래를 모두 만족하면 Buy.

1. 시장 필터: 지수 봉이 `index_ma_days`개 이상 있고, 지수 당일 종가 > 지수 종가 `index_ma_days`일 이동평균(당일 포함)
2. 봉이 `ma_long_days + 1`개 이상 있음
3. 당일 거래대금(종가 × 거래량) ≥ `min_turnover`
4. 당일 거래대금 ≥ 직전 `turnover_ma_days`일 평균 거래대금(당일 제외) × `turnover_ratio_min`
5. 종가 ≥ 당일 고가 × `close_to_high_min`
6. `change_min` ≤ (종가 / 전일 종가 − 1) < `change_max`
7. 종가 > `ma_short_days`일 종가 이동평균 > `ma_long_days`일 종가 이동평균 (모두 당일 포함)
8. 종가 ≥ 직전 `new_high_days`일(당일 제외) 고가의 최댓값

`Score`는 당일 거래대금 / 직전 `turnover_ma_days`일 평균 거래대금.

**청산:**

- `OnOpen`: `open ≥ EntryPrice × (1 + gap_hold_min)` 이고 `!pos.Extended` 이면 Hold(연장), 아니면 Sell. 갭 하락도 이 규칙에 따라 시가에 매도된다 (별도 손절 규칙 없음).
- `OnClose`(보유 중): 연장한 날이면 Sell(당일 종가 매도). 연장하지 않은 보유 상태는 `OnOpen`에서 이미 팔렸으므로 발생하지 않는다.

결과적으로 보유 기간은 1거래일(익일 시가 매도) 또는 2거래일(연장 후 당일 종가 매도)이다.

## 9. 백테스트 엔진

### 9.1 루프

전 종목의 날짜 집합을 합쳐 오름차순으로 순회한다. 날짜 `d`마다:

1. **시가 단계.** 보유 종목마다 `d`의 봉이 있으면 `OnOpen` 호출. Sell이면 `d` 시가에 매도 체결. Hold이면 `pos.Extended = true`. `d`의 봉이 없는 종목(거래정지)은 다음 날로 넘긴다.
2. **종가 단계 - 청산.** `d`의 봉이 있는 보유 종목마다 `OnClose` 호출. Sell이면 `d` 종가 × (1 − `close_slippage`)에 매도 체결.
3. **종가 단계 - 진입.** `d`의 봉이 있는 미보유 종목마다, 그 종목 시장의 지수 봉을 `d`까지 잘라 `Market`으로 넘기고 `OnClose` 호출. `d`의 지수 봉이 없으면 그날은 그 시장 종목 전부 진입하지 않는다. Buy 후보를 `Score` 내림차순 정렬, 상위 `max_entries_per_day`개를 `d` 종가 × (1 + `close_slippage`)에 매수 체결.
4. **평가.** `d` 종가 기준으로 현금 + 보유 평가액을 자산 곡선에 기록.

### 9.2 체결과 비용

- 매수 수량 = floor(총자산 × `position_pct` / 체결가). 현금이 모자라면 floor(현금 / 체결가). 0주면 진입하지 않는다.
- 매수 비용 = 체결가 × 수량 × (1 + `commission_rate`)
- 매도 수익 = 체결가 × 수량 × (1 − `commission_rate` − `tax_rate`)
- 총자산 = 현금 + Σ(보유 수량 × 당일 종가). 진입 시 사용하는 총자산은 그날 진입 전 기준으로 고정한다 (같은 날 여러 종목 진입해도 비율이 일정).

### 9.3 룩어헤드 방지

전략에 넘기는 슬라이스는 항상 `bars[:i+1]`과 `IndexBars[:j+1]`(둘 다 마지막이 `d`)이며, `OnOpen`에 넘기는 `bars`는 전날까지의 봉(`bars[:i]`)이다. 당일 시가만 별도 인자로 준다.

## 10. 관심 종목 선별 (internal/screener)

전일까지의 봉만으로 "오늘 종가 C와 거래량 V가 얼마 이상이면 8.2절 진입 조건을 통과하는가"를 종목마다 계산한다. 실전 운영의 1단계 사전 선별이자 TUI 관심 종목 화면의 데이터다. 네트워크를 쓰지 않는다.

### 10.1 문턱값 계산

전일까지의 봉이 `ma_long_days`개 이상 있는 종목만 대상으로 한다. 기호: `P` = 전일 종가, `S19` = 직전 `ma_short_days − 1`일 종가 합, `S59` = 직전 `ma_long_days − 1`일 종가 합, `H20` = 직전 `new_high_days`일 고가 최댓값, `T20` = 직전 `turnover_ma_days`일 평균 거래대금.

| 조건 (8.2절 번호) | 오늘 종가 C 의 하한 | 비고 |
|---|---|---|
| 6. 상승률 하한 | `P × (1 + change_min)` | |
| 6. 상승률 상한 | 상한: `C < P × (1 + change_max)` | 실현 가능 여부 판정에 사용 |
| 8. 신고가 | `H20` | |
| 7. 종가 > 20일선 | `S19 / (ma_short_days − 1)` | `C > (S19 + C)/20 ⇔ C > S19/19` |
| 7. 20일선 > 60일선 | `(S59 − 3·S19) / 2` (20/60일 기준) | 일반식: `C > (n·S59 − m·S19) / (m − n)`, `m = ma_long_days`, `n = ma_short_days` 을 `(S_{m−1}+C)/m < (S_{n−1}+C)/n` 에서 유도 |
| 3·4. 거래대금 | 거래대금 하한 `T_req = max(min_turnover, turnover_ratio_min × T20)` | 거래량 하한은 `T_req / C_min` 으로 표시 |

`C_min` = 위 하한들의 최댓값. `C_min < P × (1 + change_max)` 이면 **실현 가능**, 아니면 오늘 통과 불가능하므로 목록에서 뺀다.

1(시장 필터)과 5(마감 강도)는 당일 값이 있어야 하므로 문턱값이 없다. 시장 필터는 전일 지수 종가 대비 20일선 위치를 참고 정보로 함께 보여준다.

### 10.2 출력

```go
type WatchItem struct {
    Code, Name, Market string
    PrevClose          int64
    MinClose           int64   // C_min
    MinChangePct       float64 // C_min / P − 1
    MinTurnover        int64   // T_req
    MinVolume          int64   // T_req / C_min
    Thresholds         map[string]int64 // 조건별 하한 (상세 화면용)
}
```

`MinChangePct` 오름차순으로 정렬한다 (통과에 필요한 상승폭이 작은 순).

### 10.3 검증

문턱값 대수식이 맞는지, 무작위 봉을 만들어 "C = C_min 일 때 8.2절 조건(1, 5 제외)을 만족하고 C = C_min − 1 일 때 하나 이상 실패"를 확인하는 테스트로 검증한다. 이 테스트는 `strategy` 패키지의 실제 조건 함수를 호출한다.

## 11. TUI (internal/tui)

bubbletea 모델 하나가 현재 화면 상태를 들고, 화면마다 하위 모델을 둔다. 로직은 전부 다른 패키지에 있고 TUI는 호출과 표시만 한다. 키: `↑↓` 이동, `Enter` 선택, `Esc` 이전 화면, `q` 종료.

| 화면 | 내용 | 데이터 출처 |
|---|---|---|
| 대시보드 (첫 화면) | 코스피·코스닥 전일 종가와 20일선, 필터 통과 여부, 관심 종목 수, 데이터 최신 날짜, 메뉴 | `data`, `screener` |
| 관심 종목 | `WatchItem` 표. 종목명, 시장, 전일 종가, 필요 최소 종가와 상승률, 필요 거래량. Enter로 상세 | `screener` |
| 종목 상세 | 최근 20일 봉 표, 20·60일선, 20일 고가, 조건별 문턱값과 전일 종가 대비 거리 | `data`, `screener` |
| 전략 | 진입 조건 8개와 설정값, 청산 규칙, 비용·사이징. 읽기 전용 | `config` |
| 지수 | 코스피·코스닥 최근 60일 종가 ASCII 차트 + 20일선, 탭으로 전환 | `data` |
| 수집 | 유니버스 갱신 / 일봉 수집 선택 → 진행률 바, 현재 종목, 실패 목록. 완료 후 대시보드 갱신 | `kis`, `data` |
| 백테스트 실행 | 시작일·종료일 입력 → 실행 → 진행률(날짜 기준) | `backtest` |
| 결과 | 상단 지표 요약, 중단 자산 곡선 ASCII 차트, 하단 거래 내역 표(스크롤). 가장 최근 결과는 `out/`에서 다시 불러옴 | `report` |

**장시간 작업**(수집, 백테스트)은 `tea.Cmd`로 고루틴에서 실행하고, 진행 상황을 채널로 받아 메시지로 변환한다. 실행 중에도 화면이 멈추지 않으며 `Esc`로 취소(컨텍스트 취소)할 수 있다.

**터미널 크기**: 최소 80×24를 가정한다. 표는 남는 높이만큼만 그리고 나머지는 스크롤한다.

**테스트**: 각 화면 모델에 메시지를 보내 상태 전이를 검증한다 (메뉴 선택 → 화면 전환, 진행 메시지 → 진행률 갱신, 취소). 렌더링 문자열은 핵심 문구 포함 여부만 확인한다.

## 12. 리포트

터미널 요약:

| 지표 | 정의 |
|---|---|
| 총수익률 | 최종 자산 / 초기 자산 − 1 |
| CAGR | (최종/초기)^(365/일수) − 1 |
| MDD | 자산 곡선 최고점 대비 최대 낙폭 |
| 샤프 | 일별 수익률 평균 / 표준편차 × √252 (무위험 0) |
| 승률 | 손익 > 0 거래 / 전체 거래 |
| 손익비 | 평균 이익 / 평균 손실 |
| 거래 수, 평균 보유일, 연장 비율 | |
| 시장 필터로 진입 차단된 일수 | 필터 효과 확인용 |

파일 출력 (`--out` 디렉터리, 기본 `out/`):

- `trades.csv`: code, name, market, entry_date, entry_price, qty, exit_date, exit_price, pnl, pnl_pct, extended
- `equity.csv`: date, cash, holdings_value, total

리포트 하단에 "유니버스와 제외 플래그는 현재 시점 기준이라 생존 편향이 있음"을 명시한다.

## 13. 에러 처리

- `fetch`: 종목 하나가 실패해도 계속 진행하고, 끝에 실패 종목과 사유를 출력한다. 종목 단위로 트랜잭션을 걸어 부분 저장을 막는다. 지수 일봉은 종목보다 먼저 받고, 실패하면 즉시 중단한다 (시장 필터가 동작하지 않으므로).
- 401: 토큰 재발급 후 1회 재시도. 그래도 실패하면 즉시 중단 (앱키 문제).
- 초당 제한 초과: 1초 대기 후 최대 3회 재시도.
- `backtest`: 봉이 없는 종목은 경고 출력 후 건너뛴다. 지수 봉이 하나도 없으면 시작 전에 실패한다. 설정값이 범위를 벗어나면(비율 음수, `ma_short_days >= ma_long_days` 등) 시작 전에 실패한다.
- 앱키 환경변수가 없으면 `universe`, `fetch`는 시작 전에 실패한다. `backtest`, `watch`는 앱키 없이 동작한다. TUI는 앱키 없이 시작되며, 수집 화면 진입 시에만 앱키 부재를 안내한다.
- TUI에서 수집·백테스트가 실패하면 오류 메시지를 화면에 표시하고 대시보드로 돌아간다. 프로그램은 종료하지 않는다.

## 14. 테스트

TDD로 진행한다. 한투 실서버는 테스트에서 호출하지 않는다.

- **strategy**: 조건별 경계값 테스트. 지수 정확히 20일선 위/아래, 거래대금 정확히 100억과 3배, 종가/고가 정확히 0.99, 상승률 정확히 3%와 20%, 20일선과 60일선 동일가, 신고가 동일가. 연장 조건 갭 정확히 2%. 각 조건이 단독으로 실패할 때 Buy가 나오지 않는지.
- **backtest**: 손으로 계산 가능한 3~5일 가짜 봉으로 검증. 종가 매수 + 슬리피지 + 수수료, 익일 시가 매도 + 세금, 갭 연장 후 종가 매도, `max_entries_per_day` 초과 시 Score 순 선택, 현금 부족 시 수량 축소, 거래정지(봉 없음) 건너뛰기, 지수 봉 없는 날 진입 차단, 자산 곡선 값.
- **룩어헤드**: 가짜 전략으로 넘어온 `bars`와 `IndexBars` 마지막 날짜가 항상 현재 날짜 이하인지 검증.
- **kis**: `httptest.Server`로 100건 분할 호출, 빈 응답 종료, 401 재발급 재시도, 초당 제한 재시도, 지수 응답 파싱 검증. 마스터 파일 파서는 실제 파일에서 잘라낸 몇 줄(정상 종목, 관리종목, SPAC)로 검증.
- **data**: 임시 SQLite에 저장/조회 왕복, upsert 중복 처리, `LastBarDate`, 지수 봉 왕복.
- **report**: 알려진 자산 곡선으로 MDD, CAGR, 샤프 검증.
- **screener**: 10.3절의 대수식 검증. 실현 불가능 종목 제외, 정렬 순서.
- **tui**: 11절의 화면 모델 상태 전이 테스트. 실제 터미널은 띄우지 않는다.

## 15. 나중 단계 (이번 범위 아님)

1. 투자자별 매매동향 수집 → 외국인·기관 순매수 진입 조건
2. 모의투자 계좌로 매일 장 마감 전 자동 실행 (스케줄러 + 주문 API). 15:15경 거래대금 순위 API로 후보를 좁힌 뒤 후보만 REST 현재가 스냅샷으로 조건 검사, 15:20 전 주문. 웹소켓·분봉은 불필요
3. 파라미터 스캔 (설정값 격자 탐색)
4. 상장폐지 종목과 관리종목 지정 이력을 포함한 유니버스 이력
5. 장중 손절을 위한 웹소켓 실시간 체결가

## 부록. 실전 단계 메모 (이번 범위 아님)

설계 논의 중 나온 실전 운영 관련 결론. 실전 단계 설계 때 출발점으로 쓴다.

### A.1 지수 데이터는 REST로 충분하다

- 백테스트는 과거 지수 일봉이 필요하므로 REST 일봉 API(7.3절) 외에 대안이 없다.
- 실전에서도 시장 필터는 하루 한 번(15:20 전) 판단하므로 REST 현재가 조회 한 번이면 된다.
- 웹소켓 실시간 업종지수(TR `H0UPCNT0`)는 "보유 중 지수 급락 시 장중 청산" 같은 장중 규칙을 넣을 때만 필요하다.

### A.2 종목도 분봉·실시간 스트림이 필요 없다

- 종가 베팅 판단에 필요한 것은 15:19경 스냅샷(시가, 고가, 현재가, 누적 거래량) 한 장이며, REST 현재가 조회가 이를 한 번에 준다.
- 한투 웹소켓은 연결당 구독 41건 제한이 있어 전 종목 실시간 수신은 애초에 불가능하다.
- 한투는 과거 분봉을 당일분만 제공하므로 수년치 분봉 백테스트는 불가능하다. 일봉 종가 + 슬리피지로 근사한다.
- 15:19 가격과 실제 종가는 동시호가 동안 달라진다. 백테스트의 `close_slippage`가 이를 흉내 내며, 실전 체결가와 비교해 보정한다.

### A.3 2,500종목 스냅샷 문제: 사전 선별 3단계

진입 조건(8.2절) 대부분은 "오늘 종가가 X 이상"이라는 문턱값으로 전날 데이터에서 미리 계산된다.

1. **전날 밤 관심 종목 추리기 (로컬 DB만 사용).** 10절의 `screener`가 이 단계이며 이번 범위에 포함된다. 경험적으로 100~300종목이 남는다. 지수가 20일선에서 크게 벗어나 있으면 그날은 쉰다.
2. **15:10 관심 종목 스냅샷.** 남은 종목을 REST 현재가로 훑는다 (200종목 × 초당 15건 = 약 15초). 문턱값 비교로 10~30종목으로 줄인다.
3. **15:18 생존 종목 재확인 후 주문.** 마감 강도(현재가 ≥ 고가 × `close_to_high_min`)까지 확인하고 `Score` 상위 `max_entries_per_day`개에 주문. 이 단계 종목은 41건 이하이므로 원하면 웹소켓 구독으로 마지막 몇 분을 볼 수 있다.

보조 수단: 한투 거래량·거래대금 순위 API로 1단계에서 놓친 당일 급등 종목을 안전망으로 잡을 수 있다. 호출당 반환 건수 제한이 있어 메인 경로로는 부족하다.

문턱값 계산은 전략 조건 그 자체이므로 백테스트의 `strategy` 패키지를 그대로 재사용한다.
