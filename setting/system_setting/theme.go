package system_setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type ThemeSettings struct {
	Frontend string `json:"frontend"`
}

var themeSettings = ThemeSettings{
	// 【修改说明 - 2026-07-03】
	// 修改背景：新部署默认需要直接使用新版前端，而不是经典前端。
	// 解决问题：将系统主题默认配置改为 default，启动后同步到 common 主题状态。
	// 设计考虑：只调整默认值，不移除 classic 选项，避免影响后台手动切换能力。
	// 注意事项：已有数据库如果已经保存 theme.frontend=classic，需要在后台或数据库中改为 default。
	Frontend: "default",
}

func init() {
	config.GlobalConfig.Register("theme", &themeSettings)
	syncThemeToCommon()
}

func syncThemeToCommon() {
	common.SetTheme(themeSettings.Frontend)
}

func GetThemeSettings() *ThemeSettings {
	return &themeSettings
}

// UpdateAndSyncTheme syncs the theme config to common after DB load.
func UpdateAndSyncTheme() {
	syncThemeToCommon()
}
