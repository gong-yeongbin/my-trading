# 국내 주식 종가 베팅 백테스트 시스템 설계

작성일: 2026-09-13

## 1. 목표와 범위

개인 프로그램 매매를 위한 첫 단계로, 국내 주식 일봉 기반 **종가 베팅 전략의 백테스트 환경**을 Go로 만든다.

이번 범위에 포함:

- 한국투자증권(한투) Open API로 일봉 수집, 로컬 SQLite 저장
- 한투 종목 마스터 파일로 유니버스 구성
- 종가 베팅 전략 1개 (조건부 보유 연장 포함)
- 이벤트 기반 일봉 백테스트 엔진 (체결, 수수료, 거래세, 슬리피지)
- 성과 리포트 (요약 지표, 거래 내역 CSV, 자산 곡선 CSV)

이번 범위에서 제외 (나중 단계):

- 모의투자/실전 주문
- 실시간 시세 (웹소켓, 틱)
- 장중 손절/익절
- 공매도, 레버리지, 분할 매수
- 차트 출력
- 상장폐지 종목 반영 (생존 편향 보정)

## 2. 결정 사항 요약

| 항목 | 결정 | 이유 |
|---|---|---|
| 시장 | 국내 주식, 일봉 | 원화 단일, 나중에 실전도 같은 API |
| 언어 | Go 단일 | 실전 봇 확장 시 동시성·배포 유리. 백테스트 성능은 충분 |
| 데이터 소스 | 한투 Open API | 공식 지원, 수정주가 제공, 실전까지 하나의 API |
| 저장소 | SQLite (`modernc.org/sqlite`, CGO 불필요) | 백테스트가 네트워크 없이 로컬만 읽도록 |
| 유니버스 | 한투 마스터 파일, 코스피+코스닥 보통주 전체 | 종가 베팅은 스캔형이라 넓은 유니버스 필요 |
| 전략 스타일 | 종가 베팅 (종가 매수, 익일 시가 매도, 조건부 연장) | 사용자 선택 |
| 체결 모델 | 매수는 당일 종가+슬리피지, 매도는 익일 시가 또는 당일 종가 | 일봉만으로 재현 가능 |

## 3. 디렉터리 구조

```
my-trading/
  cmd/trader/main.go          CLI 진입점: universe, fetch, backtest
  internal/config/            config.yaml 로딩, 환경변수(앱키) 읽기
  internal/kis/               한투 API 클라이언트: 토큰, 일봉 조회, 호출 제한, 마스터 파일
  internal/data/              Bar 타입, Store 인터페이스, SQLite 구현
  internal/strategy/          Strategy 인터페이스, 종가 베팅 전략
  internal/backtest/          엔진, 포트폴리오, 체결/비용 모델
  internal/report/            지표 계산, 터미널 출력, CSV 출력
  config.yaml                 종목 필터, 전략 파라미터, 비용, 사이징
  data/market.db              SQLite (git 제외)
  docs/superpowers/specs/     설계 문서
```

외부 의존성은 SQLite 드라이버와 YAML 파서(`gopkg.in/yaml.v3`)로 제한한다.

## 4. CLI

```
trader universe            마스터 파일 다운로드 → 필터 적용 → symbols 테이블 갱신
trader fetch [--from DATE] symbols 테이블의 종목 일봉을 증분 수집
trader backtest [--from DATE] [--to DATE]   SQLite만 읽어 백테스트 실행, 리포트 출력
```

`fetch`와 `universe`만 네트워크를 쓴다. `backtest`는 네트워크를 쓰지 않는다.

`fetch`는 종목마다 `LastBarDate` 다음 날부터 오늘까지 받는다. 저장된 봉이 없으면 `--from`(없으면 `fetch.start_date`)부터 받는다.

`data/`, `out/`은 `.gitignore`에 넣는다.

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

fetch:
  start_date: "2021-01-01"     # 최초 수집 시작일

strategy:
  volume_ma_days: 20
  volume_ratio_min: 2.0        # 당일 거래량 >= 20일 평균 × 2
  price_ma_days: 20            # 종가 > 20일 이동평균
  change_min: 0.03             # 전일 대비 상승률 하한
  change_max: 0.25             # 상한 (상한가 근처 제외)
  min_avg_turnover: 1000000000 # 20일 평균 거래대금 하한 (원)
  gap_hold_min: 0.01           # 익일 시가 갭 >= +1% 면 보유 연장

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

### 6.1 Bar

```go
type Bar struct {
    Date   time.Time // 영업일, KST 자정
    Open   int64     // 원 단위 정수
    High   int64
    Low    int64
    Close  int64
    Volume int64
}
```

가격은 원 단위 정수로 다룬다. 국내 주식은 소수점 가격이 없고, 부동소수 오차를 피한다. 수정주가로 받으므로 별도 보정은 없다.

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
```

### 6.3 Store 인터페이스

```go
type Store interface {
    UpsertSymbols(ctx context.Context, syms []Symbol) error
    ListSymbols(ctx context.Context) ([]Symbol, error)
    LastBarDate(ctx context.Context, code string) (time.Time, bool, error)
    UpsertBars(ctx context.Context, code string, bars []Bar) error
    LoadBars(ctx context.Context, code string, from, to time.Time) ([]Bar, error)
}
```

## 7. 한투 API 클라이언트 (internal/kis)

공식 GitHub 예제(`koreainvestment/open-trading-api`)에서 확인한 사양을 따른다.

### 7.1 토큰

- `POST {base_url}/oauth2/tokenP`, body: `grant_type=client_credentials`, `appkey`, `appsecret`
- 응답의 `access_token`, `access_token_token_expired`(만료 일시)를 `token_cache` 파일에 저장
- 만료 전이면 파일의 토큰을 재사용한다. 재발급은 분당 1회 제한이므로 매 실행마다 발급하면 안 된다.
- API 호출이 401을 돌려주면 토큰을 한 번 재발급하고 같은 요청을 한 번 재시도한다.

### 7.2 일봉 조회

- `GET /uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice`
- 헤더: `authorization: Bearer {token}`, `appkey`, `appsecret`, `tr_id: FHKST03010100`
- 파라미터: `FID_COND_MRKT_DIV_CODE=J`, `FID_INPUT_ISCD={code}`, `FID_INPUT_DATE_1={시작 YYYYMMDD}`, `FID_INPUT_DATE_2={종료 YYYYMMDD}`, `FID_PERIOD_DIV_CODE=D`, `FID_ORG_ADJ_PRC=0`(수정주가)
- 응답 `output2` 배열의 필드: `stck_bsop_date`, `stck_oprc`, `stck_hgpr`, `stck_lwpr`, `stck_clpr`, `acml_vol`
- **호출당 최대 100건.** 요청 기간을 종료일에서 거꾸로 140일(달력일 기준, 영업일 약 100일) 단위로 잘라 반복 호출하고, 응답이 비거나 시작일 이전에 도달하면 멈춘다. 응답에 빈 문자열 가격이 섞인 행(거래정지 등)은 건너뛴다.

### 7.3 호출 제한

- 토큰 버킷 방식 rate limiter로 `requests_per_second`를 지킨다.
- 초당 제한 초과 에러(`EGW00201`)를 받으면 1초 대기 후 같은 요청을 재시도한다 (최대 3회).

### 7.4 마스터 파일

- `https://new.real.download.dws.co.kr/common/master/kospi_code.mst.zip`, `kosdaq_code.mst.zip` 다운로드 (인증 불필요)
- 압축 해제 후 CP949 → UTF-8 변환, 한 줄이 한 종목
- 앞부분: 단축코드(9자, 앞 6자 사용), 표준코드(12자), 한글명(가변). 뒷부분 228자(코스피) 고정폭 필드에 `그룹코드`(ST=보통주) 등이 있다. 코스닥은 뒷부분 폭이 다르므로 공식 예제(`stocks_info/kis_kospi_code_mst.py`, `kis_kosdaq_code_mst.py`)의 필드 폭을 그대로 옮긴다.
- `universe.group_codes` 필터를 적용한 결과를 `symbols` 테이블에 저장한다.

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

type Strategy interface {
    // 장 마감 시점. bars 는 해당 종목의 당일까지 일봉(과거→현재), 마지막이 당일.
    // pos 가 nil 이면 미보유: Buy 또는 Hold 를 반환.
    // pos 가 있으면 보유 중: Sell(당일 종가 매도) 또는 Hold 를 반환.
    OnClose(bars []Bar, pos *Position) Action

    // 익일 시가 시점. 보유 종목에 대해서만 호출. open 은 당일 시가.
    // Sell(시가 매도) 또는 Hold(보유 연장) 를 반환.
    OnOpen(bars []Bar, open int64, pos *Position) Action

    // 같은 날 Buy 후보가 max_entries_per_day 를 넘을 때 우선순위. 클수록 우선.
    Score(bars []Bar) float64
}
```

전략은 미래 봉을 절대 받지 않는다. 엔진이 `bars[:i+1]`만 넘긴다.

### 8.2 종가 베팅 전략 규칙

**진입 (OnClose, 미보유):** 아래를 모두 만족하면 Buy.

- 봉이 `volume_ma_days + 1`개 이상 있음
- 당일 거래량 ≥ 직전 `volume_ma_days`일 평균 거래량 × `volume_ratio_min` (당일 제외 평균)
- 종가 > 시가
- 종가 > 직전 `price_ma_days`일 종가 이동평균 (당일 포함)
- `change_min` ≤ (종가 / 전일 종가 − 1) < `change_max`
- 직전 20일 평균 거래대금(종가 × 거래량) ≥ `min_avg_turnover`

`Score`는 당일 거래량 / 20일 평균 거래량.

**청산:**

- `OnOpen`: `open ≥ EntryPrice × (1 + gap_hold_min)` 이고 `!pos.Extended` 이면 Hold(연장), 아니면 Sell.
- `OnClose`(보유 중): 연장한 날이면 Sell(당일 종가 매도). 연장하지 않은 보유 상태는 `OnOpen`에서 이미 팔렸으므로 발생하지 않는다.

결과적으로 보유 기간은 1거래일(익일 시가 매도) 또는 2거래일(연장 후 당일 종가 매도)이다.

## 9. 백테스트 엔진

### 9.1 루프

전 종목의 날짜 집합을 합쳐 오름차순으로 순회한다. 날짜 `d`마다:

1. **시가 단계.** 보유 종목마다 `d`의 봉이 있으면 `OnOpen` 호출. Sell이면 `d` 시가에 매도 체결. Hold이면 `pos.Extended = true`. `d`의 봉이 없는 종목(거래정지)은 다음 날로 넘긴다.
2. **종가 단계 - 청산.** `d`의 봉이 있는 보유 종목마다 `OnClose` 호출. Sell이면 `d` 종가 × (1 − `close_slippage`)에 매도 체결.
3. **종가 단계 - 진입.** `d`의 봉이 있는 미보유 종목마다 `OnClose` 호출. Buy 후보를 `Score` 내림차순 정렬, 상위 `max_entries_per_day`개를 `d` 종가 × (1 + `close_slippage`)에 매수 체결.
4. **평가.** `d` 종가 기준으로 현금 + 보유 평가액을 자산 곡선에 기록.

### 9.2 체결과 비용

- 매수 수량 = floor(총자산 × `position_pct` / 체결가). 현금이 모자라면 floor(현금 / 체결가). 0주면 진입하지 않는다.
- 매수 비용 = 체결가 × 수량 × (1 + `commission_rate`)
- 매도 수익 = 체결가 × 수량 × (1 − `commission_rate` − `tax_rate`)
- 총자산 = 현금 + Σ(보유 수량 × 당일 종가). 진입 시 사용하는 총자산은 그날 진입 전 기준으로 고정한다 (같은 날 여러 종목 진입해도 비율이 일정).

### 9.3 룩어헤드 방지

전략에 넘기는 슬라이스는 항상 `bars[:i+1]`이며, `OnOpen`에 넘기는 `bars`는 전날까지의 봉(`bars[:i]`)이다. 당일 시가만 별도 인자로 준다.

## 10. 리포트

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

파일 출력 (`--out` 디렉터리, 기본 `out/`):

- `trades.csv`: code, name, entry_date, entry_price, qty, exit_date, exit_price, pnl, pnl_pct, extended
- `equity.csv`: date, cash, holdings_value, total

리포트 하단에 "유니버스는 현재 상장 종목 기준이라 생존 편향이 있음"을 명시한다.

## 11. 에러 처리

- `fetch`: 종목 하나가 실패해도 계속 진행하고, 끝에 실패 종목과 사유를 출력한다. 종목 단위로 트랜잭션을 걸어 부분 저장을 막는다.
- 401: 토큰 재발급 후 1회 재시도. 그래도 실패하면 즉시 중단 (앱키 문제).
- 초당 제한 초과: 1초 대기 후 최대 3회 재시도.
- `backtest`: 봉이 없는 종목은 경고 출력 후 건너뛴다. 설정값이 범위를 벗어나면(비율 음수 등) 시작 전에 실패한다.
- 앱키 환경변수가 없으면 `universe`, `fetch`는 시작 전에 실패한다. `backtest`는 앱키 없이 동작한다.

## 12. 테스트

TDD로 진행한다. 한투 실서버는 테스트에서 호출하지 않는다.

- **strategy**: 경계값 테스트. 거래량 정확히 2배, 상승률 정확히 3%와 25%, 이동평균 동일가, 거래대금 하한. 연장 조건 갭 정확히 1%.
- **backtest**: 손으로 계산 가능한 3~5일 가짜 봉으로 검증. 종가 매수 + 슬리피지 + 수수료, 익일 시가 매도 + 세금, 갭 연장 후 종가 매도, `max_entries_per_day` 초과 시 Score 순 선택, 현금 부족 시 수량 축소, 거래정지(봉 없음) 건너뛰기, 자산 곡선 값.
- **룩어헤드**: 가짜 전략으로 넘어온 `bars` 마지막 날짜가 항상 현재 날짜 이하인지 검증.
- **kis**: `httptest.Server`로 100건 분할 호출, 빈 응답 종료, 401 재발급 재시도, 초당 제한 재시도 검증. 마스터 파일 파서는 실제 파일에서 잘라낸 몇 줄로 검증.
- **data**: 임시 SQLite에 저장/조회 왕복, upsert 중복 처리, `LastBarDate`.
- **report**: 알려진 자산 곡선으로 MDD, CAGR, 샤프 검증.

## 13. 나중 단계 (이번 범위 아님)

1. 모의투자 계좌로 매일 장 마감 전 자동 실행 (스케줄러 + 주문 API)
2. 파라미터 스캔 (설정값 격자 탐색)
3. 상장폐지 종목 포함한 유니버스 이력
4. 장중 손절을 위한 웹소켓 실시간 체결가
