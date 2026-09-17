package settings

import "testing"

func TestDisplayMasksSecrets(t *testing.T) {
	if got := Display(KISDemoKey, "PSabcdefghijklmnop"); got != "PSab****" {
		t.Errorf("key mask = %q", got)
	}
	if got := Display(KISDemoSecret, "abc"); got != "****" {
		t.Errorf("short secret = %q", got)
	}
	if got := Display(KISDemoKey, ""); got != "(없음)" {
		t.Errorf("empty = %q", got)
	}
	if got := Display(KISEnv, "demo"); got != "demo" {
		t.Errorf("plain = %q", got)
	}
	if got := Display(DailyAt, ""); got != "(없음)" {
		t.Errorf("empty plain = %q", got)
	}
}

func TestIsSecret(t *testing.T) {
	for _, f := range []Field{KISDemoKey, KISDemoSecret, KISRealKey, KISRealSecret, LSKey, LSSecret} {
		if !IsSecret(f) {
			t.Errorf("%s should be secret", Labels[f])
		}
	}
	for _, f := range []Field{KISEnv, KISDemoAccount, KISRealAccount, DailyAt, StartDate} {
		if IsSecret(f) {
			t.Errorf("%s should not be secret", Labels[f])
		}
	}
}

func TestValidate(t *testing.T) {
	ok := map[Field][]string{
		KISEnv:         {"demo", "real"},
		DailyAt:        {"04:00", "23:59"},
		StartDate:      {"2025-09-01"},
		KISDemoAccount: {"", "12345678-01"},
		KISDemoKey:     {"", "PSabc123"},
	}
	bad := map[Field][]string{
		KISEnv:         {"", "paper"},
		DailyAt:        {"4:00", "24:00", ""},
		StartDate:      {"2025/09/01", ""},
		KISDemoAccount: {"1234567801", "12345678-1"},
		KISDemoKey:     {"has space"},
	}
	for f, vals := range ok {
		for _, v := range vals {
			if err := Validate(f, v); err != nil {
				t.Errorf("%s %q should be valid: %v", Labels[f], v, err)
			}
		}
	}
	for f, vals := range bad {
		for _, v := range vals {
			if err := Validate(f, v); err == nil {
				t.Errorf("%s %q should be invalid", Labels[f], v)
			}
		}
	}
}

func TestGetSetRoundTrip(t *testing.T) {
	var v Values
	for f := Field(0); f < FieldCount; f++ {
		v.Set(f, "x"+Labels[f])
	}
	for f := Field(0); f < FieldCount; f++ {
		if v.Get(f) != "x"+Labels[f] {
			t.Errorf("field %d round trip", f)
		}
	}
}
