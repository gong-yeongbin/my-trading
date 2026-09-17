// Package settings 는 TUI 설정 메뉴가 다루는 값의 정의·표시·검증과 .env / config.yaml 읽기쓰기를 맡는다.
package settings

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Field int

const (
	KISEnv Field = iota
	KISDemoKey
	KISDemoSecret
	KISDemoAccount
	KISRealKey
	KISRealSecret
	KISRealAccount
	LSKey
	LSSecret
	DailyAt
	StartDate
	FieldCount
)

var Labels = [FieldCount]string{
	"매매 서버", "한투 모의 앱키", "한투 모의 시크릿", "한투 모의 계좌",
	"한투 실전 앱키", "한투 실전 시크릿", "한투 실전 계좌",
	"LS 앱키", "LS 시크릿", "자동 수집 시각", "수집 시작일",
}

// Values 는 설정 메뉴의 모든 값. 문자열 그대로 보관한다.
type Values struct {
	KISEnv, KISDemoKey, KISDemoSecret, KISDemoAccount string
	KISRealKey, KISRealSecret, KISRealAccount         string
	LSKey, LSSecret, DailyAt, StartDate               string
}

func (v Values) Get(f Field) string {
	switch f {
	case KISEnv:
		return v.KISEnv
	case KISDemoKey:
		return v.KISDemoKey
	case KISDemoSecret:
		return v.KISDemoSecret
	case KISDemoAccount:
		return v.KISDemoAccount
	case KISRealKey:
		return v.KISRealKey
	case KISRealSecret:
		return v.KISRealSecret
	case KISRealAccount:
		return v.KISRealAccount
	case LSKey:
		return v.LSKey
	case LSSecret:
		return v.LSSecret
	case DailyAt:
		return v.DailyAt
	case StartDate:
		return v.StartDate
	}
	return ""
}

func (v *Values) Set(f Field, s string) {
	switch f {
	case KISEnv:
		v.KISEnv = s
	case KISDemoKey:
		v.KISDemoKey = s
	case KISDemoSecret:
		v.KISDemoSecret = s
	case KISDemoAccount:
		v.KISDemoAccount = s
	case KISRealKey:
		v.KISRealKey = s
	case KISRealSecret:
		v.KISRealSecret = s
	case KISRealAccount:
		v.KISRealAccount = s
	case LSKey:
		v.LSKey = s
	case LSSecret:
		v.LSSecret = s
	case DailyAt:
		v.DailyAt = s
	case StartDate:
		v.StartDate = s
	}
}

// IsSecret 은 화면에서 가려야 하는 필드.
func IsSecret(f Field) bool {
	switch f {
	case KISDemoKey, KISDemoSecret, KISRealKey, KISRealSecret, LSKey, LSSecret:
		return true
	}
	return false
}

// Display 는 목록에 보일 문자열. 비밀은 앞 4자만 남기고 가린다.
func Display(f Field, v string) string {
	if v == "" {
		return "(없음)"
	}
	if !IsSecret(f) {
		return v
	}
	r := []rune(v)
	if len(r) <= 4 {
		return "****"
	}
	return string(r[:4]) + "****"
}

var (
	accountRe = regexp.MustCompile(`^\d{8}-\d{2}$`)
	tokenRe   = regexp.MustCompile(`^\S+$`)
)

// Validate 는 저장 전 값 검사.
func Validate(f Field, v string) error {
	switch f {
	case KISEnv:
		if v != "demo" && v != "real" {
			return errors.New("demo 또는 real")
		}
	case DailyAt:
		if _, err := time.Parse("15:04", v); err != nil || len(v) != 5 {
			return errors.New("HH:MM 형식")
		}
	case StartDate:
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return errors.New("YYYY-MM-DD 형식")
		}
	case KISDemoAccount, KISRealAccount:
		if v != "" && !accountRe.MatchString(v) {
			return errors.New("12345678-01 형식")
		}
	default:
		if v != "" && !tokenRe.MatchString(strings.TrimSpace(v)) {
			return fmt.Errorf("공백 없는 문자열")
		}
	}
	return nil
}
