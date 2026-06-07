package controller

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const chuamgweiInternalSecretHeader = "X-Chuamgwei-Internal-Secret"

type ChuamgweiUserSyncRequest struct {
	UserUuid  string `json:"userUuid" binding:"required"`
	Username  string `json:"username"`
	IsBanned  int    `json:"isBanned"`
	IsDeleted int    `json:"isDeleted"`
}

// SyncChuamgweiUser 供 Node 用户主系统同步 new-api 影子用户和默认 API Key。
func SyncChuamgweiUser(c *gin.Context) {
	if !verifyChuamgweiInternalSecret(c) {
		return
	}

	var req ChuamgweiUserSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	userUuid := strings.TrimSpace(req.UserUuid)
	if userUuid == "" {
		common.ApiErrorMsg(c, "用户UUID不能为空")
		return
	}

	enabled := req.IsBanned == 0 && req.IsDeleted == 0
	shadowUser, userCreated, err := model.UpsertChuamgweiShadowUser(userUuid, req.Username, enabled)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	token, tokenCreated, err := model.EnsureChuamgweiDefaultToken(
		shadowUser.Id,
		common.GetChuamgweiDefaultTokenGroup(),
		common.GetChuamgweiDefaultTokenRemainQuota(),
		common.IsChuamgweiDefaultTokenUnlimited(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"userId":            shadowUser.Id,
		"username":          shadowUser.Username,
		"status":            shadowUser.Status,
		"userCreated":       userCreated,
		"chuamgweiUserUuid": shadowUser.ChuamgweiUserUuid,
		"tokenId":           token.Id,
		"tokenName":         token.Name,
		"tokenCreated":      tokenCreated,
		"key":               token.GetFullKey(),
		"maskedKey":         token.GetMaskedKey(),
		"unlimitedQuota":    token.UnlimitedQuota,
		"remainQuota":       token.RemainQuota,
	})
}

// verifyChuamgweiInternalSecret 校验 Node 调用 new-api 内部同步接口的服务间密钥。
func verifyChuamgweiInternalSecret(c *gin.Context) bool {
	expectedSecret := common.GetChuamgweiUserSyncSecret()
	if expectedSecret == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "chuamgwei 用户同步密钥未配置",
		})
		c.Abort()
		return false
	}

	actualSecret := c.GetHeader(chuamgweiInternalSecretHeader)
	if actualSecret == "" || subtle.ConstantTimeCompare([]byte(actualSecret), []byte(expectedSecret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "内部同步认证失败",
		})
		c.Abort()
		return false
	}
	return true
}
