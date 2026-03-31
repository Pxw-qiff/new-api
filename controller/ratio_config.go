package controller

import (
	"net/http"
	"sort"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func GetRatioConfig(c *gin.Context) {
	if !ratio_setting.IsExposeRatioEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "倍率配置接口未启用",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    ratio_setting.GetExposedData(),
	})
}

type modelBillingItem struct {
	ID          string `json:"id"`
	BillingType string `json:"billing_type"`
}

// GetModelBillingList 返回所有开启了渠道的模型及其计费方式（按量/按次）
func GetModelBillingList(c *gin.Context) {
	enabledModels := model.GetEnabledModels()
	priceMap := ratio_setting.GetModelPriceCopy()

	items := make([]modelBillingItem, 0, len(enabledModels))

	for _, name := range enabledModels {
		if _, ok := priceMap[name]; ok {
			items = append(items, modelBillingItem{
				ID:          name,
				BillingType: "per-request",
			})
		} else {
			items = append(items, modelBillingItem{
				ID:          name,
				BillingType: "per-token",
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// GetModelBillingListByVendor 根据供应商返回模型列表及计费信息
func GetModelBillingListByVendor(c *gin.Context) {
	vendorName := c.Param("vendor")
	if vendorName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "参数错误：缺少供应商名称",
		})
		return
	}

	// 1. 查询供应商 ID
	var vendorID int
	if err := model.DB.Table("vendors").Select("id").Where("name = ?", vendorName).Scan(&vendorID).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "查询供应商出错",
		})
		return
	}
	if vendorID == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "供应商不存在",
		})
		return
	}

	// 2. 获取该供应商下建立的所有模型（不限制模型广场里的 status=1，因为即使隐藏，只要添加了渠道就能用）
	var vendorModels []string
	if err := model.DB.Table("models").Select("model_name").Where("vendor_id = ?", vendorID).Scan(&vendorModels).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "查询模型报错",
		})
		return
	}

	// 过滤：必须是系统中实际可用的模型
	enabledModels := model.GetEnabledModels()
	enabledMap := make(map[string]bool)
	for _, em := range enabledModels {
		enabledMap[em] = true
	}

	var models []string
	for _, vm := range vendorModels {
		if enabledMap[vm] {
			models = append(models, vm)
		}
	}

	// 3. 构建返回计费项
	priceMap := ratio_setting.GetModelPriceCopy()

	items := make([]modelBillingItem, 0, len(models))

	for _, name := range models {
		if _, ok := priceMap[name]; ok {
			items = append(items, modelBillingItem{
				ID:          name,
				BillingType: "per-request",
			})
		} else {
			items = append(items, modelBillingItem{
				ID:          name,
				BillingType: "per-token",
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}
