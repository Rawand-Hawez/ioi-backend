package handlers

import (
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

// numericToRat converts a pgtype.Numeric to *big.Rat using exact integer
// arithmetic (no float64 intermediary). Returns zero-rat for NULL/invalid.
func numericToRat(n *pgtype.Numeric) *big.Rat {
	if n.Int == nil {
		return big.NewRat(0, 1)
	}
	rat := new(big.Rat).SetInt(n.Int)
	if n.Exp > 0 {
		mul := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil)
		rat.Mul(rat, new(big.Rat).SetInt(mul))
	} else if n.Exp < 0 {
		div := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-n.Exp)), nil)
		rat.Quo(rat, new(big.Rat).SetInt(div))
	}
	return rat
}

// numericToBigRat converts a pgtype.Numeric (value receiver) to *big.Rat
// using the same exact-integer path as numericToRat.
func numericToBigRat(n pgtype.Numeric) (*big.Rat, error) {
	if !n.Valid {
		return big.NewRat(0, 1), nil
	}
	// Re-use pointer version
	p := n
	return numericToRat(&p), nil
}

// roundRatToCents rounds a *big.Rat to the nearest cent using half-up
// rounding and returns the total number of cents (int64).
func roundRatToCents(amount *big.Rat) int64 {
	scaled := new(big.Rat).Mul(amount, big.NewRat(100, 1))
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(scaled.Num(), scaled.Denom(), remainder)
	// Half-up: if remainder*2 >= denominator, round up
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(scaled.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient.Int64()
}

// formatCents converts a cent count to a "X.XX" string.
func formatCents(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// formatMoney2 converts a *big.Rat to a "X.XX" string using half-up rounding.
// This replaces the old truncating formatMoney2 (R8-1 fix).
func formatMoney2(amount *big.Rat) string {
	cents := roundRatToCents(amount)
	if cents < 0 {
		cents = 0
	}
	return formatCents(cents)
}

// ratToNumeric converts a *big.Rat back to a pgtype.Numeric via string scan.
func ratToNumeric(r *big.Rat) pgtype.Numeric {
	var num pgtype.Numeric
	floatStr := r.FloatString(18)
	num.Scan(floatStr)
	return num
}

// negateNumeric returns a negated copy of a pgtype.Numeric.
func negateNumeric(n *pgtype.Numeric) pgtype.Numeric {
	if n.Int == nil || !n.Valid {
		return pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
	}
	return pgtype.Numeric{
		Int:   new(big.Int).Neg(n.Int),
		Exp:   n.Exp,
		Valid: true,
	}
}

// formatContractAmount formats a *big.Rat as a fixed-point decimal with 2 places.
func formatContractAmount(amount *big.Rat) string {
	return amount.FloatString(2)
}
