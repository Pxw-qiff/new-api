package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

const (
	chuamgweiCreditBizType        = "NEW_API_CHAT"
	chuamgweiCreditSecretHeader   = "X-Internal-Secret"
	chuamgweiCreditDefaultBaseURL = "http://127.0.0.1:8080"
)

type chuamgweiCreditEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type chuamgweiCreditBillingPayload struct {
	UserUuid        string `json:"userUuid"`
	BizType         string `json:"bizType"`
	BizOrderNo      string `json:"bizOrderNo"`
	EstimatedPoints string `json:"estimatedPoints,omitempty"`
	ActualPoints    string `json:"actualPoints,omitempty"`
	Remark          string `json:"remark,omitempty"`
}

type chuamgweiCreditAccount struct {
	AvailablePoints decimal.Decimal `json:"availablePoints"`
	FrozenPoints    decimal.Decimal `json:"frozenPoints"`
}

func IsChuamgweiCreditEnabled() bool {
	return common.IsChuamgweiCreditEnabled()
}

func GetChuamgweiCreditAvailableQuota(userUuid string) (int, error) {
	if strings.TrimSpace(userUuid) == "" {
		return 0, fmt.Errorf("chuamgwei 用户UUID为空")
	}
	var account chuamgweiCreditAccount
	endpoint := "/internal/credit/balance?userUuid=" + url.QueryEscape(userUuid)
	if err := callChuamgweiCredit(http.MethodGet, endpoint, nil, &account); err != nil {
		return 0, err
	}
	return creditPointsToQuota(account.AvailablePoints), nil
}

func PreConsumeChuamgweiCredit(userUuid, bizOrderNo string, quota int, remark string) error {
	if quota <= 0 {
		return nil
	}
	payload := chuamgweiCreditBillingPayload{
		UserUuid:        userUuid,
		BizType:         chuamgweiCreditBizType,
		BizOrderNo:      bizOrderNo,
		EstimatedPoints: quotaToCreditPoints(quota).StringFixed(6),
		Remark:          remark,
	}
	return callChuamgweiCredit(http.MethodPost, "/internal/credit/pre-consume", payload, nil)
}

func SettleChuamgweiCredit(userUuid, bizOrderNo string, actualQuota int, remark string) error {
	if actualQuota < 0 {
		actualQuota = 0
	}
	payload := chuamgweiCreditBillingPayload{
		UserUuid:     userUuid,
		BizType:      chuamgweiCreditBizType,
		BizOrderNo:   bizOrderNo,
		ActualPoints: quotaToCreditPoints(actualQuota).StringFixed(6),
		Remark:       remark,
	}
	return callChuamgweiCredit(http.MethodPost, "/internal/credit/settle", payload, nil)
}

func RefundChuamgweiCredit(userUuid, bizOrderNo string, remark string) error {
	payload := chuamgweiCreditBillingPayload{
		UserUuid:   userUuid,
		BizType:    chuamgweiCreditBizType,
		BizOrderNo: bizOrderNo,
		Remark:     remark,
	}
	return callChuamgweiCredit(http.MethodPost, "/internal/credit/refund", payload, nil)
}

func callChuamgweiCredit(method, endpoint string, payload any, out any) error {
	baseURL := strings.TrimRight(common.GetEnvOrDefaultString("CHUAMGWEI_CREDIT_BASE_URL", chuamgweiCreditDefaultBaseURL), "/")
	if baseURL == "" {
		return fmt.Errorf("CHUAMGWEI_CREDIT_BASE_URL 未配置")
	}
	internalSecret := common.GetEnvOrDefaultString("CHUAMGWEI_CREDIT_INTERNAL_SECRET", "")
	if internalSecret == "" {
		return fmt.Errorf("CHUAMGWEI_CREDIT_INTERNAL_SECRET 未配置")
	}

	var body io.Reader
	if payload != nil {
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(bodyBytes)
	}

	ctx, cancel := context.WithTimeout(context.Background(), chuamgweiCreditTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, baseURL+"/"+strings.TrimLeft(endpoint, "/"), body)
	if err != nil {
		return err
	}
	req.Header.Set(chuamgweiCreditSecretHeader, internalSecret)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: chuamgweiCreditTimeout()}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("积分系统返回异常: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var envelope chuamgweiCreditEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("积分系统响应解析失败: %w", err)
	}
	if envelope.Code != http.StatusOK {
		return fmt.Errorf("积分系统返回失败: code=%d, message=%s", envelope.Code, envelope.Message)
	}
	if out != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("积分系统数据解析失败: %w", err)
		}
	}
	return nil
}

func chuamgweiCreditTimeout() time.Duration {
	timeoutMs := common.GetEnvOrDefault("CHUAMGWEI_CREDIT_TIMEOUT_MS", 5000)
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

func quotaToCreditPoints(quota int) decimal.Decimal {
	if quota <= 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(quota)).Mul(chuamgweiCreditPointsPerQuota()).Round(6)
}

func creditPointsToQuota(points decimal.Decimal) int {
	if points.LessThanOrEqual(decimal.Zero) {
		return 0
	}
	return int(points.Div(chuamgweiCreditPointsPerQuota()).IntPart())
}

func chuamgweiCreditPointsPerQuota() decimal.Decimal {
	rawValue := strings.TrimSpace(common.GetEnvOrDefaultString("CHUAMGWEI_CREDIT_POINTS_PER_QUOTA", "1"))
	pointsPerQuota, err := decimal.NewFromString(rawValue)
	if err != nil || pointsPerQuota.LessThanOrEqual(decimal.Zero) {
		return decimal.NewFromInt(1)
	}
	return pointsPerQuota
}
