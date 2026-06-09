package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestChuamgweiCreditDefaultPointsPerQuotaUsesDisplayAmount(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	t.Setenv("CHUAMGWEI_CREDIT_POINTS_PER_QUOTA", "")

	ratio := decimal.NewFromInt(1)

	require.Equal(t, "0.001508", quotaToCreditPoints(754, ratio).StringFixed(6))
	require.Equal(t, "0.002000", quotaToCreditPoints(1000, ratio).StringFixed(6))
	require.Equal(t, 754, creditPointsToQuota(decimal.RequireFromString("0.001508"), ratio))
	require.Equal(t, 500000, creditPointsToQuota(decimal.RequireFromString("1"), ratio))
}

func TestChuamgweiCreditChargeRatioMultipliesPointsPerQuota(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	t.Setenv("CHUAMGWEI_CREDIT_POINTS_PER_QUOTA", "")

	ratio := decimal.NewFromInt(2)

	require.Equal(t, "0.004000", quotaToCreditPoints(1000, ratio).StringFixed(6))
	require.Equal(t, "2.000000", quotaToCreditPoints(500000, ratio).StringFixed(6))
	require.Equal(t, 500000, creditPointsToQuota(decimal.RequireFromString("2"), ratio))
}

func TestChuamgweiCreditPointsPerQuotaCanBeOverridden(t *testing.T) {
	t.Setenv("CHUAMGWEI_CREDIT_POINTS_PER_QUOTA", "0.01")
	ratio := decimal.NewFromInt(1)

	require.Equal(t, "7.540000", quotaToCreditPoints(754, ratio).StringFixed(6))
	require.Equal(t, 754, creditPointsToQuota(decimal.RequireFromString("7.54"), ratio))
}
