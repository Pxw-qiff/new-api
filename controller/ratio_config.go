package controller

import (
	"net/http"
	"sort"

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
	ID              string  `json:"id"`
	BillingType     string  `json:"billing_type"`
	InputRatio      float64 `json:"input_ratio,omitempty"`
	CompletionRatio float64 `json:"completion_ratio,omitempty"`
	CacheRatio      float64 `json:"cache_ratio,omitempty"`
	Price           float64 `json:"price,omitempty"`
}

// GetModelBillingList 返回所有模型及其计费方式（按量/按次）和相关价格信息
func GetModelBillingList(c *gin.Context) {
	if !ratio_setting.IsExposeRatioEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "倍率配置接口未启用",
		})
		return
	}

	ratioMap := ratio_setting.GetModelRatioCopy()
	priceMap := ratio_setting.GetModelPriceCopy()
	completionRatioMap := ratio_setting.GetCompletionRatioCopy()
	cacheRatioMap := ratio_setting.GetCacheRatioCopy()

	items := make([]modelBillingItem, 0, len(ratioMap)+len(priceMap))

	// 按量计费模型
	for name, ratio := range ratioMap {
		item := modelBillingItem{
			ID:          name,
			BillingType: "per-token",
			InputRatio:  ratio,
		}
		if cr, ok := completionRatioMap[name]; ok {
			item.CompletionRatio = cr
		}
		if car, ok := cacheRatioMap[name]; ok {
			item.CacheRatio = car
		}
		items = append(items, item)
	}

	// 按次计费模型
	for name, price := range priceMap {
		items = append(items, modelBillingItem{
			ID:          name,
			BillingType: "per-request",
			Price:       price,
		})
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
	if !ratio_setting.IsExposeRatioEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "倍率配置接口未启用",
		})
		return
	}

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

	// 2. 获取该供应商下所有开启的模型
	var models []string
	if err := model.DB.Table("models").Select("model_name").Where("vendor_id = ? AND status = 1", vendorID).Scan(&models).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "查询模型报错",
		})
		return
	}

	// 3. 构建返回计费项
	ratioMap := ratio_setting.GetModelRatioCopy()
	priceMap := ratio_setting.GetModelPriceCopy()
	completionRatioMap := ratio_setting.GetCompletionRatioCopy()
	cacheRatioMap := ratio_setting.GetCacheRatioCopy()

	items := make([]modelBillingItem, 0, len(models))

	for _, name := range models {
		// 先查是否带有计费价格（按次计费）
		if price, ok := priceMap[name]; ok {
			items = append(items, modelBillingItem{
				ID:          name,
				BillingType: "per-request",
				Price:       price,
			})
			continue
		}

		// 再查按量计费
		if ratio, ok := ratioMap[name]; ok {
			item := modelBillingItem{
				ID:          name,
				BillingType: "per-token",
				InputRatio:  ratio,
			}
			if cr, ok := completionRatioMap[name]; ok {
				item.CompletionRatio = cr
			}
			if car, ok := cacheRatioMap[name]; ok {
				item.CacheRatio = car
			}
			items = append(items, item)
			continue
		}

		// 默认取 37.5 加上可能的补全倍率 (参照 GetModelRatioOrPrice 逻辑)
		item := modelBillingItem{
			ID:          name,
			BillingType: "per-token",
			InputRatio:  37.5, // 默认倍率
		}
		if cr, ok := completionRatioMap[name]; ok {
			item.CompletionRatio = cr
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}
