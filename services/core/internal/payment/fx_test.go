package payment

import (
	"testing"
)

func TestFXToBaseUnits(t *testing.T) {
	tests := []struct {
		name      string
		usdAmount float64
		decimals  int
		fiatPerUSD float64
		want      string
	}{
		// ARS: $0.02 * 1050 ARS/USD = 21 ARS = 21e18 base units.
		// 21e19 overflows int64 (max ~9.2e18) — verifies big.Int is used.
		{
			name: "wARS $0.02 at 1050 ARS/USD",
			usdAmount: 0.02, decimals: 18, fiatPerUSD: 1050.0,
			want: "21000000000000000000",
		},
		// BRL: $0.02 * 5.8 BRL/USD = 0.116 BRL = 116e15 base units. Fits int64.
		{
			name: "wBRL $0.02 at 5.8 BRL/USD",
			usdAmount: 0.02, decimals: 18, fiatPerUSD: 5.8,
			want: "116000000000000000",
		},
		// COP: $0.02 * 4100 COP/USD = 82 COP = 82e18 base units.
		// 82e18 overflows int64 and uint64 — verifies big.Int is used.
		{
			name: "wCOP $0.02 at 4100 COP/USD",
			usdAmount: 0.02, decimals: 18, fiatPerUSD: 4100.0,
			want: "82000000000000000000",
		},
		// USDC-like: sanity check that it agrees with DollarsToBaseUnits for 6 decimals.
		{
			name: "USDC-like $0.02 at 1.0",
			usdAmount: 0.02, decimals: 6, fiatPerUSD: 1.0,
			want: "20000",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FXToBaseUnits(tc.usdAmount, tc.decimals, tc.fiatPerUSD)
			if got != tc.want {
				t.Errorf("FXToBaseUnits(%v, %d, %v) = %q, want %q",
					tc.usdAmount, tc.decimals, tc.fiatPerUSD, got, tc.want)
			}
		})
	}
}
