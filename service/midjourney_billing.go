package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func GetMidjourneyWalletQuota(userId int) (int, string, error) {
	if !IsChuamgweiCreditEnabled() {
		quota, err := model.GetUserQuota(userId, false)
		return quota, "", err
	}
	userUuid, err := model.GetChuamgweiUserUuid(userId)
	if err != nil {
		return 0, "", err
	}
	quota, err := GetChuamgweiCreditAvailableQuota(userUuid)
	return quota, userUuid, err
}

func BuildMidjourneyCreditBizOrderNo(relayInfo *relaycommon.RelayInfo) string {
	if relayInfo == nil {
		return ""
	}
	return strings.TrimSpace(relayInfo.RequestId)
}

func PreConsumeMidjourneyCredit(relayInfo *relaycommon.RelayInfo, userUuid string, bizOrderNo string, quota int, action string) error {
	if !IsChuamgweiCreditEnabled() || quota <= 0 {
		return nil
	}
	if relayInfo == nil {
		return fmt.Errorf("relayInfo 为空，无法预扣 Midjourney 积分")
	}
	if strings.TrimSpace(userUuid) == "" || strings.TrimSpace(bizOrderNo) == "" {
		return fmt.Errorf("Midjourney 积分用户或业务单号为空")
	}
	if err := PreConsumeTokenQuota(relayInfo, quota); err != nil {
		return err
	}
	remark := fmt.Sprintf("Midjourney 任务预扣积分，action=%s，requestId=%s", action, bizOrderNo)
	if err := PreConsumeChuamgweiCredit(userUuid, bizOrderNo, quota, remark); err != nil {
		rollbackMidjourneyTokenQuota(relayInfo, quota)
		return err
	}
	return nil
}

func MidjourneyUsesExternalCredit(task *model.Midjourney) bool {
	return IsChuamgweiCreditEnabled() && task != nil && task.ChuamgweiUserUuid != "" && task.CreditBizOrderNo != ""
}

func SettleMidjourneyQuota(ctx context.Context, task *model.Midjourney, reason string) {
	if !MidjourneyUsesExternalCredit(task) || task.Quota <= 0 {
		return
	}
	remark := fmt.Sprintf("Midjourney 任务完成结算，mjId=%s，reason=%s", task.MjId, reason)
	if err := SettleChuamgweiCredit(task.ChuamgweiUserUuid, task.CreditBizOrderNo, task.Quota, remark); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Midjourney 外部积分结算失败 mjId=%s: %s", task.MjId, err.Error()))
	}
}

func RefundMidjourneyQuota(ctx context.Context, task *model.Midjourney, reason string) {
	if task == nil || task.Quota <= 0 {
		return
	}
	if MidjourneyUsesExternalCredit(task) {
		remark := fmt.Sprintf("Midjourney 任务失败退款，mjId=%s，reason=%s", task.MjId, reason)
		if err := RefundChuamgweiCredit(task.ChuamgweiUserUuid, task.CreditBizOrderNo, remark); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Midjourney 外部积分退款失败 mjId=%s: %s", task.MjId, err.Error()))
			return
		}
	} else if err := model.IncreaseUserQuota(task.UserId, task.Quota, false); err != nil {
		logger.LogError(ctx, "fail to increase user quota: "+err.Error())
		return
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: CovertMjpActionToModelName(task.Action),
		Quota:     task.Quota,
		Other: map[string]interface{}{
			"task_id": task.MjId,
			"reason":  reason,
		},
	})
}

func rollbackMidjourneyTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) {
	if relayInfo == nil || relayInfo.IsPlayground || quota <= 0 || relayInfo.TokenId <= 0 || relayInfo.TokenKey == "" {
		return
	}
	if err := model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota); err != nil {
		common.SysLog("error rolling back Midjourney token quota: " + err.Error())
	}
}
