# 국내 주식 종가 베팅 운영 도구 설계

작성일: 2026-09-13 (2차 개정: 진입·청산 규칙, 지수 필터, 유니버스 플래그 필터 반영. 3차 개정: 관심 종목 선별과 TUI 추가. 4차 개정: TUI를 한 화면으로 단순화, LS증권 실시간 뉴스·지수, 보유 종목, 로그 추가. 백테스트 엔진·리포트를 범위에서 제외)

## 1. 목표와 범위

개인 프로그램 매매를 위한 첫 단계로, 국내 주식 일봉을 모아 **종가 베팅 전략의 관심 종목을 매일 골라 보여주는 운영 도구**를 Go로 만든다.

이번 범위에 포함:

- 한국투자증권(한투) Open API로 종목 일봉과 지수 일봉 수집, 로컬 SQLite 저장
- 한투 종목 마스터 파일로 유니버스 구성 (관리종목·거래정지 등 플래그 제외)
- 종가 베팅 전략 1개의 진입 조건 함수 (시장 필터, 유동성, 마감 강도, 상승률, 추세, 신고가)
- 관심 종목 선별: 전일 데이터로 "오늘 진입 조건을 통과하려면 필요한 종가·거래량" 문턱값 계산
- 한투 계좌 잔고 조회 (보유 종목 표시용, 읽기 전용)
- LS증권 Open API 웹소켓으로 실시간 뉴스 제목·지수 수신
- 전체 화면 TUI 한 장: 실시간 뉴스·지수, 관심 종목, 보유 종목, 로그

이번 범위에서 제외 (나중 단계):

- 백테스트 엔진과 성과 리포트
- 모의투자/실전 주문
- 실시간 종목 시세 (틱, 호가)
- 장중 손절/익절
- 투자자별 수급(외국인·기관 순매수) 조건
- 공매도, 레버리지, 분할 매수
- 차트 출력
- 상장폐지 종목, 과거 시점의 관리종목 지정 이력 반영 (생존 편향 보정)

## 2. 결정 사항 요약

| 항목 | 결정 | 이유 |
|---|---|---|
| 시장 | 국내 주식, 일봉 | 원화 단일, 나중에 실전도 같은 API |
| 언어 | Go 단일 | 실전 봇 확장 시 동시성·배포 유리 |
| 데이터 소스 | 한투 Open API | 공식 지원, 수정주가 제공, 실전까지 하나의 API |
| 저장소 | SQLite (`modernc.org/sqlite`, CGO 불필요) | 선별과 TUI가 네트워크 없이 로컬만 읽도록 |
| 유니버스 | 한투 마스터 파일, 코스피+코스닥 보통주 전체, 플래그 종목 제외 | 종가 베팅은 스캔형이라 넓은 유니버스 필요 |
| 전략 스타일 | 종가 베팅 (종가 매수, 익일 시가 매도, 조건부 연장) | 사용자 선택 |
| 시장 필터 | 종목이 속한 시장의 지수(코스피 0001 / 코스닥 1001)가 20일선 위일 때만 진입 | 하락장 갭 하락 회피 |
| 화면 | 전체 화면 TUI (bubbletea) 한 장 + 비대화형 서브커맨드 병행 | 매일 보는 운영 대시보드 용도. 수집은 서브커맨드로 |
| 실시간 데이터 | LS증권 Open API 웹소켓 (뉴스 `NWS`, 업종지수 `IJ`) | 한투는 뉴스 웹소켓이 없음. 폴링 대신 푸시로 받음 |
| 시세 서버 | 항상 실전 (KIS_REAL_*) | 모의 서버는 시세 조회가 느리고, 시세 값은 모의·실전 동일 |

## 3. 디렉터리 구조

```
my-trading/
  cmd/trader/main.go          CLI 진입점: universe, fetch, watch, TUI
  internal/config/            config.yaml 로딩, 환경변수(앱키) 읽기
  internal/kis/               한투 API 클라이언트: 토큰, 종목/지수 일봉, 호출 제한, 마스터 파일, 잔고 조회
  internal/ls/                LS증권 웹소켓 클라이언트: 토큰, 실시간 뉴스·지수·장운영정보 구독, 재연결
  internal/logfile/           JSON 줄 로그 파일 열기·파싱·꼬리 읽기 (서브커맨드·TUI 공용)
  internal/market/            장 상태 판정: 시계 기준, JIF 코드표 (순수 함수)
  internal/settings/          설정 메뉴 값 정의·검증, .env·config.yaml 읽기쓰기 (줄 단위 편집, 원자적 저장)
  internal/data/              Bar·IndexBar 타입, Store 인터페이스, SQLite 구현
  internal/strategy/          종가 베팅 진입 조건 함수
  internal/screener/          관심 종목 선별: 전일 봉으로 진입 문턱값 계산
  internal/tui/               bubbletea 전체 화면 하나. 다른 패키지를 호출만 하고 로직은 갖지 않음
  config.yaml                 종목 필터, 전략 파라미터, 비용, 사이징
  data/market.db              SQLite (git 제외)
  data/trader.log             서브커맨드·TUI 공용 로그, JSON 한 줄씩 (git 제외)
  docs/superpowers/specs/     설계 문서
```

외부 의존성은 SQLite 드라이버, YAML 파서(`gopkg.in/yaml.v3`), 마스터 파일 CP949 디코딩용 `golang.org/x/text`, TUI용 `charmbracelet/bubbletea`·`bubbles`·`lipgloss`, LS 웹소켓용 `github.com/coder/websocket`(구 nhooyr.io/websocket)으로 제한한다. `data/`, `.env`는 `.gitignore`에 넣는다.

## 4. CLI

```
trader                     인자 없이 실행하면 TUI 시작
trader universe            마스터 파일 다운로드 → 필터 적용 → symbols 테이블 갱신
trader fetch [--from DATE] symbols 테이블의 종목 일봉 + 코스피/코스닥 지수 일봉을 증분 수집
trader watch               전일 데이터 기준 관심 종목을 표로 출력
```

`fetch`와 `universe`만 한투 데이터 API를 쓴다. `watch`는 네트워크를 쓰지 않는다. TUI는 SQLite를 읽고, 실시간 뉴스·지수는 LS 웹소켓으로, 보유 종목은 한투 잔고 조회로 받는다. TUI 가 켜져 있으면 매일 `fetch.daily_at`(기본 04:00 KST)에 전날까지의 일봉을 자동 수집하고, 켤 때 지수 마지막 봉이 직전 평일보다 오래됐으면 즉시 한 번 받는다. 자동 수집은 어제까지만 요청해 장중의 미완성 봉을 저장하지 않는다. `fetch` 서브커맨드는 수동·스크립트용이다.

`fetch`는 종목(및 지수)마다 마지막 저장일 다음 날부터 오늘까지 받는다. 저장된 봉이 없으면 `--from`(없으면 `fetch.start_date`)부터 받는다.

## 5. 설정 (config.yaml)

```yaml
kis:
  trade_env: demo               # 매매·잔고(8단계)에만 적용. demo: 모의투자(openapivts...:29443, 초당 2건) / real: 실전(openapi...:9443, 초당 20건)
  # 시세(일봉·지수·마스터) 수집은 항상 실전 서버·실전 키. 모의 서버는 시세 조회가 느리고 값은 모의·실전 동일하기 때문
  # 시세 앱키·시크릿·계좌는 .env 의 KIS_REAL_APP_KEY/KIS_REAL_APP_SECRET/KIS_REAL_ACCOUNT (fallback 없음)
  # 매매 앱키·시크릿·계좌는 trade_env 에 따라 KIS_DEMO_* 또는 KIS_REAL_* (demo 는 구 이름 KIS_APP_KEY 등 fallback). TUI 설정 메뉴에서 입력 가능
  token_cache: data/token_real.json       # 시세(실전) 토큰 캐시
  trade_token_cache: data/token_demo.json # 매매용 토큰 캐시 (비어 있으면 token_cache + ".trade")
  requests_per_second: 0       # 시세 호출 제한. 0 이면 15(실전). 매매 호출 제한은 trade_env 기본값(demo 1.5, real 15) 고정
  # 계좌번호는 환경변수 KIS_REAL_ACCOUNT / KIS_DEMO_ACCOUNT ("12345678-01": 앞 8자리 CANO, 뒤 2자리 ACNT_PRDT_CD). 없으면 보유 종목 패널은 "미연결"
  balance_poll_seconds: 10     # 장중 보유 종목 갱신 주기

ls:
  base_url: https://openapi.ls-sec.co.kr:8080
  ws_url: wss://openapi.ls-sec.co.kr:9443/websocket
  # app_key, app_secret 은 .env 또는 환경변수 LS_APP_KEY, LS_APP_SECRET. 없으면 뉴스·지수 줄은 "미연결"
  token_cache: data/ls_token.json

log:
  file: data/trader.log

universe:
  markets: [kospi, kosdaq]
  group_codes: [ST]            # 보통주만
  exclude_flags: [거래정지, 정리매매, 관리종목, 시장경고, 단기과열, 이상급등, SPAC]

fetch:
  start_date: "2021-01-01"     # 최초 수집 시작일 (1차 선별 전용 운영에선 1년치면 충분)
  daily_at: "04:00"            # TUI 자동 수집 시각 (KST). 비어 있으면 04:00

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
- 응답의 `access_token`, `access_token_token_expired`(만료 일시)를 토큰 캐시 파일에 저장
- 시세(일봉·지수·마스터, 항상 실전)용과 매매·잔고(8단계, `trade_env`에 따라 모의/실전)용 캐시 파일을 분리한다 — `kis.token_cache`(시세)와 `kis.trade_token_cache`(매매, 비어 있으면 `token_cache + ".trade"`). 두 서버·키가 다를 수 있으므로 하나의 캐시를 같이 쓰면 재발급이 잦아진다.
- 캐시 파일에는 발급받은 `base_url` 도 함께 저장한다. 읽을 때 캐시의 `base_url` 이 클라이언트의 것과 다르거나(구 형식이라 필드가 없는 경우 포함) 없으면 캐시를 무시하고 새로 발급한다 — 시세/매매, 모의/실전 캐시가 파일 하나로 섞여도 안전하다.
- 만료 전이면 파일의 토큰을 재사용한다. 재발급은 분당 1회 제한이므로 매 실행마다 발급하면 안 된다.
- API 호출이 401 이거나, 응답 `msg_cd` 가 `EGW00121`/`EGW00123`(토큰 만료·오류, HTTP 상태와 무관)이면 토큰을 한 번 재발급하고 같은 요청을 한 번 재시도한다. 재시도 후에도 실패하면 포기한다.

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
- **한계:** 플래그는 현재 시점 값이다. 과거에 관리종목이었다가 해제된 종목은 포함되고, 현재 관리종목인 종목은 과거 정상 기간까지 제외된다. 1차에서는 이 근사를 감수한다.

### 7.6 잔고 조회 (보유 종목)

- `GET /uapi/domestic-stock/v1/trading/inquire-balance`
- 헤더: 일봉과 동일, `tr_id: TTTC8434R`(실전) / `VTTC8434R`(모의)
- 파라미터: `CANO`, `ACNT_PRDT_CD`, `AFHR_FLPR_YN=N`, `OFL_YN=`, `INQR_DVSN=02`, `UNPR_DVSN=01`, `FUND_STTL_ICLD_YN=N`, `FNCG_AMT_AUTO_RDPT_YN=N`, `PRCS_DVSN=01`, `CTX_AREA_FK100=`, `CTX_AREA_NK100=`
- 응답 `output1`(종목별): `pdno`, `prdt_name`, `hldg_qty`, `pchs_avg_pric`, `prpr`, `evlu_pfls_amt`, `evlu_pfls_rt`. `output2`(계좌 합계): `tot_evlu_amt`, `dnca_tot_amt`(예수금), `evlu_pfls_smtl_amt`
- 보유 수량 0인 행은 건너뛴다. 시장 구분은 `symbols` 테이블에서 찾는다.
- 보유일은 잔고 응답에 없다. 우리 프로그램의 매매 로그(나중 단계)에서 진입일을 찾아 계산하고, 없으면 `-`로 표시한다.

## 8. 종가 베팅 전략 (internal/strategy)

### 8.1 조건 함수

전략은 순수 함수 집합이다. 상태를 갖지 않고, 넘겨받은 봉만 본다.

```go
// 종목이 속한 시장의 지수 일봉. 종목 bars 와 같은 규칙으로 당일까지만 담긴다.
type Market struct {
    IndexBars []IndexBar
}

// Entry 는 8.2절 진입 조건 8개를 모두 검사한다. bars 는 해당 종목의 당일까지 일봉(과거→현재), 마지막이 당일.
func Entry(cfg config.StrategyConfig, mkt Market, bars []Bar) bool

// 조건별 함수. screener(9절)의 문턱값 검증 테스트가 개별 호출한다.
func MarketFilter(cfg config.StrategyConfig, mkt Market) bool
func Liquidity(cfg config.StrategyConfig, bars []Bar) bool
func CloseStrength(cfg config.StrategyConfig, bars []Bar) bool
func Change(cfg config.StrategyConfig, bars []Bar) bool
func Trend(cfg config.StrategyConfig, bars []Bar) bool
func NewHigh(cfg config.StrategyConfig, bars []Bar) bool
```

함수는 미래 봉을 절대 받지 않는다. 호출자가 `bars[:i+1]`, `IndexBars[:j+1]`만 넘긴다.

### 8.2 종가 베팅 전략 규칙

**진입:** 아래를 모두 만족하면 오늘 종가에 매수 대상.

1. 시장 필터: 지수 봉이 `index_ma_days`개 이상 있고, 지수 당일 종가 > 지수 종가 `index_ma_days`일 이동평균(당일 포함)
2. 봉이 `ma_long_days + 1`개 이상 있음
3. 당일 거래대금(종가 × 거래량) ≥ `min_turnover`
4. 당일 거래대금 ≥ 직전 `turnover_ma_days`일 평균 거래대금(당일 제외) × `turnover_ratio_min`
5. 종가 ≥ 당일 고가 × `close_to_high_min`
6. `change_min` ≤ (종가 / 전일 종가 − 1) < `change_max`
7. 종가 > `ma_short_days`일 종가 이동평균 > `ma_long_days`일 종가 이동평균 (모두 당일 포함)
8. 종가 ≥ 직전 `new_high_days`일(당일 제외) 고가의 최댓값

후보가 여럿이면 당일 거래대금 / 직전 `turnover_ma_days`일 평균 거래대금이 큰 순으로 우선한다.

**청산 (실전 주문 단계에서 구현, 이번 범위 아님):** 익일 시가 매도. 단 시가가 진입가 대비 +2% 이상 갭이면 그날 종가까지 한 번 연장. 별도 손절 규칙 없음. 보유 기간은 1거래일 또는 2거래일.

## 9. 관심 종목 선별 (internal/screener)

전일까지의 봉만으로 "오늘 종가 C와 거래량 V가 얼마 이상이면 8.2절 진입 조건을 통과하는가"를 종목마다 계산한다. 실전 운영의 1단계 사전 선별이자 TUI 관심종목 패널의 데이터다. 네트워크를 쓰지 않는다.

### 9.1 문턱값 계산

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

### 9.2 출력

```go
type WatchItem struct {
    Code, Name, Market string
    PrevClose          int64
    MinClose           int64   // C_min
    MinChangePct       float64 // C_min / P − 1
    MinTurnover        int64   // T_req
    MinVolume          int64   // T_req / C_min
    Thresholds         map[string]int64 // 조건별 하한 (나중 단계 종목 상세용)
}
```

`MinChangePct` 오름차순으로 정렬한다 (통과에 필요한 상승폭이 작은 순). 기준일(코스피 지수 마지막 봉 날짜)에 봉이 없는 종목(상장폐지·거래정지·수집 미완)은 목록에서 뺀다.

관찰(2026-09-17 실데이터): 상위가 저유동 우선주로 채워진다 — 가격 문턱은 3%뿐이지만 필요 거래량이 평소의 100배가 넘는 종목들. 나중 단계에서 보조 정렬키(필요거래량/평균거래량)나 유동성 필터를 검토한다.

### 9.3 검증

문턱값 대수식이 맞는지, 무작위 봉을 만들어 "C = C_min 일 때 8.2절 조건(1, 5 제외)을 만족하고 C = C_min − 1 일 때 하나 이상 실패"를 확인하는 테스트로 검증한다. 이 테스트는 `strategy` 패키지의 실제 조건 함수를 호출한다.

## 10. TUI (internal/tui)

인자 없이 실행하면 전체 화면(alt screen)으로 뜬다. 보기 전용이다. 로직은 전부 다른 패키지에 있고 TUI는 호출과 표시만 한다.

### 10.1 레이아웃

```
────────────────────────────────────────────────────────────────────────────────────────────────────
 [연합뉴스] 삼성전자, 3분기 파운드리 수주 확대 전망                                        14:32:10
────────────────────────────────────────────────────────────────────────────────────────────────────
              │
  > 관심종목  │  관심종목  (09-12 기준, 37개)                        코스피 진입가능 · 코스닥 차단
    보유종목  │
    로그      │  종목명            시장     전일종가     필요종가    필요상승     필요거래량
              │  ──────────────────────────────────────────────────────────────────────────────
              │  > 삼성전자        코스피     71,200       73,400     +3.1%       2,140,000
              │    SK하이닉스      코스피    182,000      188,100     +3.4%         620,000
              │    ...
              │
────────────────────────────────────────────────────────────────────────────────────────────────────
 코스피 2,712.40 ▲+0.8%   코스닥 782.15 ▼-0.4%                                       Tab 패널  q 종료
────────────────────────────────────────────────────────────────────────────────────────────────────
```

| 영역 | 내용 | 출처 | 갱신 |
|---|---|---|---|
| 상단 | 최신 뉴스 한 건. LS `NWS`에는 출처 필드가 없어 제목만 표시 (출처가 있으면 `[출처] 제목`). 오른쪽 끝 현재 시각 | `ls` (`NWS`) | 새 뉴스 수신 즉시 교체. 뉴스가 없으면 `연결됨 · 뉴스 대기`. 시계는 1초 |
| 왼쪽 메뉴 | 관심종목 / 보유종목 / 로그 / 설정 | | |
| 가운데 | 선택한 패널 (아래) | | |
| 하단 | `[장 상태]` + 코스피·코스닥 현재 지수와 등락률. 오른쪽 끝 키 안내 | `market`, `ls` (`IJ_`, `JIF`) | 장 상태는 시계 기준(`market.Status`: 휴장/장전/장중/동시호가/장마감/시간외), 그날 JIF 를 받았으면 그 값. 자동 수집 중이면 키 안내 앞에 `수집 중 n/N`. 지수는 수신 즉시. 미연결이면 `미연결`, 연결됐지만 값이 없으면 `연결됨 · 장외`. 실시간 값이 없고 저장소에 지수 봉이 있으면 전일 종가와 `(전일)` 표시 |

패널:

| 패널 | 제목 줄 | 표 | 출처 | 갱신 |
|---|---|---|---|---|
| 관심종목 | 기준일, 종목 수, 시장 필터 상태 (전일 지수 종가 vs 20일선 → 시장별 `진입가능` / `차단`) | 종목명, 시장, 전일종가, 필요종가, 필요상승, 필요거래량. `MinChangePct` 오름차순 | `screener`, SQLite | 시작 시 1회 + 자동 수집 완료 후 재계산 (`screener.Run`) |
| 보유종목 | 종목 수, 평가금액, 손익(금액·%), 현금 | 종목명, 시장, 수량, 매입가, 현재가, 손익, 수익률, 보유일 | `kis` 잔고 조회 (7.6절) | 장중(09:00~15:30) `balance_poll_seconds`마다, 장외는 시작 시 1회. 보유 없으면 `보유 없음` |
| 로그 | `최신순` | 시각, 종류 태그, 메시지 | `data/trader.log` (10.3절) | 파일에 줄이 추가되면 1초 안에 반영 |
| 설정 | `Enter 편집/토글 · Esc 취소`, 저장·오류 문구 | 항목 11개(매매 서버 demo/real, 한투 모의·실전 앱키·시크릿·계좌, LS 앱키·시크릿, 자동 수집 시각, 수집 시작일)와 값. 앱키·시크릿은 앞 4자+`****`, 입력 중 `*`(편집은 빈 칸에서 시작, Esc 면 기존 값 유지). 서버 항목은 Enter 로 토글 즉시 저장, 나머지는 Enter 편집 → Enter 저장(검증 실패 시 문구) / Esc 취소. 저장은 `.env`·`config.yaml`에 쓰고 `저장됨 · 재시작하면 적용됩니다` | `settings` | 시작 시 1회 읽음, 저장 시 갱신 |

키: `Tab` 메뉴↔패널 포커스 전환, `↑↓` 이동, `Enter` 메뉴에서 패널 선택(선택과 함께 포커스가 패널로 감), `q` 종료.

터미널 크기: 최소 100×24를 가정한다. 표는 남는 높이만큼만 그리고 나머지는 스크롤한다. 한글 2칸 폭은 lipgloss 폭 계산에 맡긴다.

### 10.2 LS증권 실시간 클라이언트 (internal/ls)

포털 howto-sample 과 공개 TR 카탈로그(LsApiHelper `specs/blocks.json`)에서 확인한 사양. `IJ_`의 `tr_key`만 실서버로 검증한다.

- 토큰: `POST {base_url}/oauth2/token`, `application/x-www-form-urlencoded`, `appkey`, `appsecretkey`, `grant_type=client_credentials`, `scope=oob`. 응답 `access_token`, `expires_in`(초, 보통 86400). 토큰과 만료 시각을 `token_cache`에 저장하고 만료 30초 전까지 재사용한다.
- 웹소켓 `ws_url`에 연결한 뒤 구독마다 JSON 한 건: `{"header":{"token":…,"tr_type":"3"},"body":{"tr_cd":…,"tr_key":…}}`. 해지는 `tr_type: "4"`. 20초마다 ping 을 보낸다.
- 수신 메시지: `{"header":{"tr_cd","tr_key"},"body":{…}}`. `header.tr_cd`로 분기한다. 필드 값은 전부 문자열이다.
- 구독 응답: `{"header":{"tr_cd","rsp_cd","rsp_msg"}}` (body 없음). `rsp_cd`가 `00000`이 아니면 거부다 — 경고 로그를 남기고 `SubscribeError` 이벤트로 내보낸다 (TUI 는 무시, `ls-probe` 는 출력). 정확한 코드값은 실서버 확인 후 보강한다.
- 뉴스 제목: `tr_cd: NWS`, `tr_key: NWS001`. body `date`, `time`, `id`, `title`, `code`, `realkey`, `bodysize`. 출처 필드는 없다.
- 업종 지수: `tr_cd: IJ_`, `tr_key`는 업종코드 — 코스피 종합 `001`, 코스닥 종합 `301` (xingAPI 관례. 2단계 수동 확인에서 값 크기로 검증: 코스피 수천, 코스닥 수백). body `upcode`, `jisu`(지수), `change`(전일 대비), `drate`(등락률 %, 부호 없음), `sign`(1 상한 2 상승 3 보합 4 하한 5 하락), `time`. 등락률 부호는 `sign`이 4·5 이면 음수.
- 수신 메시지는 `chan Event`(`News`, `Index`, `Connected`, `Disconnected`)로 넘기고 TUI가 `tea.Msg`로 바꾼다. 클라이언트는 TUI를 모른다.
- 연결이 끊기면 1초 후 재연결하고 실패할 때마다 간격을 2배로 늘려 최대 60초까지 기다린다 (거부·장애 시 서버를 두드리지 않기 위해). 연결에 성공했던 뒤에는 1초로 되돌린다. 재연결 후 구독을 다시 보낸다. 연결·끊김·재연결을 `*slog.Logger`로 남긴다 (파일 로거는 3단계, 그전엔 폐기 로거).
- 앱키가 없으면 클라이언트를 만들지 않고 TUI는 `미연결`로 표시한다. TUI 는 `Connected` 를 받으면 데이터가 오기 전까지 뉴스 줄 `연결됨 · 뉴스 대기`, 지수 줄 `연결됨 · 장외`로, `Disconnected` 를 받으면 전부 `미연결`로 표시한다.
- 장운영정보 `JIF`(`tr_key` 1 코스피, 2 코스닥; body `jangubun`, `jstatus`)는 `MarketStatus{Market, Code}` 이벤트로 넘긴다. TUI 는 `market.FromJIF` 코드표(11 장전, 21 장중, 31 동시호가, 41 장마감, 51·61 시간외, 52·62 장마감 — xingAPI 관례, 실서버 관측 후 보정)로 하단 장 상태를 덮어쓰고 `kind=지수` 로그를 남긴다. 표에 없는 코드는 로그만. 8단계 잔고 폴링 시간대 판정에도 쓴다.

### 10.3 로그 파일

- `log/slog` JSON 핸들러로 `log.file`에 한 줄씩 쓴다. 서브커맨드와 TUI가 같은 파일을 쓴다 (append, 줄 단위라 동시 쓰기에 안전).
- 필드: `time`, `level`, `kind`(수집 / 지수 / 매매 / 연결 / 오류), `msg`.
- TUI 로그 패널은 시작 시 마지막 200줄을 읽고, 1초마다 파일 크기를 확인해 늘어난 만큼 읽는다 (패키지 `internal/logfile`: `Open`, `Tail`, `Reader`). 로그 파일을 열 수 없으면 TUI 는 로거를 버리고 로그 패널에 `[오류] 로그 파일 열기 실패: …` 한 줄만 보인다.
- TUI 는 `ls` 클라이언트에 `kind=연결` 로거를, JIF 처리에 `kind=지수` 로거를 넘긴다. 서브커맨드는 `kind=수집` 등 자기 종류를 붙인다.
- 로테이션은 하지 않는다 (1차).

### 10.4 테스트

각 패널 모델에 메시지를 보내 상태 전이를 검증한다 (메뉴 선택 → 패널 전환, 뉴스 수신 → 상단 줄 교체, 지수 수신 → 하단 줄 갱신, 잔고 응답 → 표 갱신, `DisconnectedMsg` → `미연결` 표시). 렌더링 문자열은 핵심 문구 포함 여부만 확인한다.

## 11. 에러 처리

- `.env` 파일이 있으면 시작 시 읽어 환경변수로 올린다 (이미 설정된 환경변수가 우선). 파일이 없어도 오류가 아니다.
- `fetch`: 종목 하나가 실패해도 계속 진행하고, 끝에 실패 종목과 사유를 출력한다. 종목 단위로 트랜잭션을 걸어 부분 저장을 막는다. 지수 일봉은 종목보다 먼저 받고, 실패하면 즉시 중단한다 (시장 필터가 동작하지 않으므로).
- 401: 토큰 재발급 후 1회 재시도. 그래도 실패하면 즉시 중단 (앱키 문제).
- 초당 제한 초과: 1초 대기 후 최대 3회 재시도.
- `watch`와 TUI: 봉이 부족한 종목은 조용히 건너뛴다. 지수 봉이 하나도 없으면 시장 필터 상태를 `알 수 없음`으로 표시한다. 설정값이 범위를 벗어나면(비율 음수, `ma_short_days >= ma_long_days` 등) 모든 명령이 시작 전에 실패한다.
- 실전 키(`KIS_REAL_APP_KEY`/`KIS_REAL_APP_SECRET`)가 없으면 시세를 쓰는 `universe`, `fetch`, TUI 자동 수집은 시작 전에 실패·비활성한다(모의 fallback 없음). `watch`는 앱키 없이 동작한다. 매매 키(`trade_env`에 따른 `KIS_DEMO_*`/`KIS_REAL_*`)는 8단계(매매·잔고)에만 필요하다.
- TUI는 앱키·계좌번호 없이 시작된다. 매매 키나 `KIS_DEMO_ACCOUNT`/`KIS_REAL_ACCOUNT`(`trade_env`에 따라)가 없으면 보유 종목 패널에 `미연결`, LS 앱키가 없거나 웹소켓이 끊기면 뉴스 줄에 `미연결`, 지수 줄에 전일 종가와 `(전일)`을 표시한다. 잔고 조회 실패는 로그에 남기고 다음 주기에 다시 시도한다. 어떤 경우에도 TUI는 종료하지 않는다.
- TUI 자동 수집: 실전 앱키(`KIS_REAL_*`)나 DB 가 없으면 비활성(`kind=수집` 로그 한 줄). 지수 2건을 먼저 받아 새 봉이 없으면(주말·공휴일·이미 최신) 종목 호출을 건너뛴다. 종목 하나 실패는 로그만 남기고 계속, 인증 실패는 중단. `q` 종료 시 수집 중이던 종목 이후는 다음 실행에서 이어받는다. 잠자기 중 타이머 지연을 피하려고 1분 단위로 벽시계를 다시 잰다.
- 설정 메뉴: 저장 실패·검증 실패는 패널 제목 줄에 표시하고 TUI 는 계속. 설정 파일 읽기 실패면 편집을 막는다. 저장은 임시 파일 + rename 으로 원자적이며 config.yaml → .env 순서로 쓴다.
- 모든 서브커맨드와 TUI는 10.3절의 로그 파일에 기록한다. 서브커맨드는 파일을 열 수 없으면 stderr로 대신 쓰고 계속 진행한다. TUI 는 10.3절대로 로거를 버리고 로그 패널에 오류 한 줄을 보인다.

## 12. 테스트

TDD로 진행한다. 한투 실서버는 테스트에서 호출하지 않는다.

- **strategy**: 조건별 경계값 테스트. 지수 정확히 20일선 위/아래, 거래대금 정확히 100억과 3배, 종가/고가 정확히 0.99, 상승률 정확히 3%와 20%, 20일선과 60일선 동일가, 신고가 동일가. 각 조건이 단독으로 실패할 때 `Entry`가 false인지.
- **kis**: `httptest.Server`로 100건 분할 호출, 빈 응답 종료, 401 재발급 재시도, 초당 제한 재시도, 지수 응답 파싱, 잔고 응답 파싱(수량 0 제외) 검증. 마스터 파일 파서는 실제 파일에서 잘라낸 몇 줄(정상 종목, 관리종목, SPAC)로 검증.
- **data**: 임시 SQLite에 저장/조회 왕복, upsert 중복 처리, `LastBarDate`, 지수 봉 왕복.
- **screener**: 9.3절의 대수식 검증. 실현 불가능 종목 제외, 정렬 순서.
- **ls**: `httptest.Server` 웹소켓으로 구독 메시지 형식, `NWS`·`IJ` 수신 파싱, 끊김 후 재연결 검증.
- **tui**: 10.4절의 상태 전이 테스트. 실제 터미널은 띄우지 않는다. 로그 패널은 임시 파일에 줄을 추가해 반영되는지 검증.

## 13. 나중 단계 (이번 범위 아님)

1. 투자자별 매매동향 수집 → 외국인·기관 순매수 진입 조건
2. 모의투자 계좌로 매일 장 마감 전 자동 실행 (스케줄러 + 주문 API). 15:15경 거래대금 순위 API로 후보를 좁힌 뒤 후보만 REST 현재가 스냅샷으로 조건 검사, 15:20 전 주문. 웹소켓·분봉은 불필요
3. 백테스트 엔진·성과 리포트 (일봉 기반 체결 모델, 수수료·세금·슬리피지, MDD·CAGR·샤프), 이어서 파라미터 스캔
4. 상장폐지 종목과 관리종목 지정 이력을 포함한 유니버스 이력
5. 장중 손절을 위한 실시간 체결가 (LS 웹소켓 `S3_`/`K3_` 구독을 붙이면 됨)
6. TUI 확장: 뉴스 이력 패널, 종목 상세(최근 봉·문턱값), 로그 로테이션

## 부록. 실전 단계 메모 (이번 범위 아님)

설계 논의 중 나온 실전 운영 관련 결론. 실전 단계 설계 때 출발점으로 쓴다.

### A.1 지수 데이터는 REST로 충분하다

- 선별(9절)은 과거 지수 일봉이 필요하므로 REST 일봉 API(7.3절) 외에 대안이 없다.
- 실전에서도 시장 필터는 하루 한 번(15:20 전) 판단하므로 REST 현재가 조회 한 번이면 된다.
- 매매 판단용으로는 웹소켓 실시간 업종지수가 "보유 중 지수 급락 시 장중 청산" 같은 장중 규칙을 넣을 때만 필요하다. TUI 하단의 실시간 지수(10.2절, LS `IJ`)는 표시용이며 매매 판단에 쓰지 않는다.

### A.2 종목도 분봉·실시간 스트림이 필요 없다

- 종가 베팅 판단에 필요한 것은 15:19경 스냅샷(시가, 고가, 현재가, 누적 거래량) 한 장이며, REST 현재가 조회가 이를 한 번에 준다.
- 한투 웹소켓은 연결당 구독 41건 제한이 있어 전 종목 실시간 수신은 애초에 불가능하다.
- 한투는 과거 분봉을 당일분만 제공하므로 수년치 분봉 백테스트는 불가능하다. 나중에 백테스트를 붙일 때는 일봉 종가 + 슬리피지로 근사한다.
- 15:19 가격과 실제 종가는 동시호가 동안 달라진다. 실전 체결가를 기록해 두었다가 슬리피지 추정에 쓴다.

### A.3 2,500종목 스냅샷 문제: 사전 선별 3단계

진입 조건(8.2절) 대부분은 "오늘 종가가 X 이상"이라는 문턱값으로 전날 데이터에서 미리 계산된다.

1. **전날 밤 관심 종목 추리기 (로컬 DB만 사용).** 9절의 `screener`가 이 단계이며 이번 범위에 포함된다. 경험적으로 100~300종목이 남는다. 지수가 20일선에서 크게 벗어나 있으면 그날은 쉰다.
2. **15:10 관심 종목 스냅샷.** 남은 종목을 REST 현재가로 훑는다 (200종목 × 초당 15건 = 약 15초). 문턱값 비교로 10~30종목으로 줄인다.
3. **15:18 생존 종목 재확인 후 주문.** 마감 강도(현재가 ≥ 고가 × `close_to_high_min`)까지 확인하고 8.2절 우선순위 상위 몇 개(설정)에 주문. 이 단계 종목은 41건 이하이므로 원하면 웹소켓 구독으로 마지막 몇 분을 볼 수 있다.

보조 수단: 한투 거래량·거래대금 순위 API로 1단계에서 놓친 당일 급등 종목을 안전망으로 잡을 수 있다. 호출당 반환 건수 제한이 있어 메인 경로로는 부족하다.

문턱값 계산은 전략 조건 그 자체이므로 `strategy` 패키지의 조건 함수를 그대로 재사용한다.
