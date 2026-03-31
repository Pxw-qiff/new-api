package controller

import (
	"net/http"
	"sort"
	"strings"

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
	Type        string `json:"type"`
}

// getModelMediaType 判断模型的媒体类型，优先使用用户在后台设置的 media_type，否则自动猜测
func getModelMediaType(modelName string, mediaTypeMap map[string]string) string {
	// 优先使用用户手动设置的类型
	if mt, ok := mediaTypeMap[modelName]; ok && mt != "" {
		return mt
	}

	// 兜底：通过端点类型判断
	endpoints := model.GetModelSupportEndpointTypes(modelName)
	for _, ep := range endpoints {
		epStr := string(ep)
		if epStr == "openai-video" || strings.Contains(epStr, "kling") || strings.Contains(epStr, "jimeng") || strings.Contains(epStr, "runway") || strings.Contains(epStr, "luma") {
			return "video"
		}
		if epStr == "image-generation" || strings.Contains(epStr, "midjourney") || strings.Contains(epStr, "flux") {
			return "image"
		}
		if strings.Contains(epStr, "suno") || strings.Contains(epStr, "audio") || strings.Contains(epStr, "tts") {
			return "audio"
		}
		if epStr == "embeddings" || strings.Contains(epStr, "rerank") {
			return "embedding"
		}
	}

	// 兜底：通过模型名字关键词猜测
	lowerName := strings.ToLower(modelName)
	if strings.Contains(lowerName, "sora") || strings.Contains(lowerName, "kling") || strings.Contains(lowerName, "jimeng") || strings.Contains(lowerName, "runway") || strings.Contains(lowerName, "luma") || strings.Contains(lowerName, "veo") || strings.Contains(lowerName, "seedance") || strings.Contains(lowerName, "video") {
		return "video"
	}
	if strings.HasPrefix(lowerName, "mj_") || strings.Contains(lowerName, "midjourney") || strings.Contains(lowerName, "dall-e") || strings.Contains(lowerName, "flux") || strings.Contains(lowerName, "imagen") || strings.Contains(lowerName, "image") || strings.Contains(lowerName, "seedream") || strings.Contains(lowerName, "swap_face") {
		return "image"
	}
	if strings.Contains(lowerName, "suno") || strings.Contains(lowerName, "tts") || strings.Contains(lowerName, "whisper") || strings.Contains(lowerName, "fish") {
		return "audio"
	}
	if strings.Contains(lowerName, "embedding") || strings.Contains(lowerName, "rerank") || strings.HasPrefix(lowerName, "bge-") {
		return "embedding"
	}

	return "text"
}

// loadMediaTypeMap 批量查询 models 表中用户设置的 media_type
func loadMediaTypeMap() map[string]string {
	type row struct {
		ModelName string
		MediaType string
	}
	var rows []row
	model.DB.Table("models").Select("model_name, media_type").Where("media_type != ''").Scan(&rows)
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.ModelName] = r.MediaType
	}
	return m
}

// GetModelBillingList 返回所有开启了渠道的模型及其计费方式（按量/按次）
func GetModelBillingList(c *gin.Context) {
	enabledModels := model.GetEnabledModels()
	priceMap := ratio_setting.GetModelPriceCopy()
	mediaTypeMap := loadMediaTypeMap()

	items := make([]modelBillingItem, 0, len(enabledModels))

	for _, name := range enabledModels {
		billingType := "per-token"
		if _, ok := priceMap[name]; ok {
			billingType = "per-request"
		}
		items = append(items, modelBillingItem{
			ID:          name,
			BillingType: billingType,
			Type:        getModelMediaType(name, mediaTypeMap),
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

	// 2. 获取该供应商下建立的所有模型
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
	mediaTypeMap := loadMediaTypeMap()

	items := make([]modelBillingItem, 0, len(models))

	for _, name := range models {
		billingType := "per-token"
		if _, ok := priceMap[name]; ok {
			billingType = "per-request"
		}
		items = append(items, modelBillingItem{
			ID:          name,
			BillingType: billingType,
			Type:        getModelMediaType(name, mediaTypeMap),
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

