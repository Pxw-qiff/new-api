package apimart

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// TaskAdaptor 将本地视频任务协议适配到 Global Gateway 上游异步视频接口。
//
// 【修改说明 - 2026-07-02】
// 修改背景：本次只需要把 Global Gateway 作为供应商接入，不能新增 Global Gateway 对外客户端接口。
// 解决问题：在任务适配层完成 /v1/videos/generations 提交和 /v1/tasks/{task_id} 查询，保持 new-api 对外协议不变。
// 设计考虑：文本等 OpenAI-compatible 能力复用 openai.Adaptor，只有 Global Gateway 异步视频任务单独建 TaskAdaptor。
// 注意事项：Global Gateway 图片生成仍是异步任务，本适配器只处理视频任务，避免破坏现有同步图片链路。
type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

type videoGenerationRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Duration    int      `json:"duration,omitempty"`
	Resolution  string   `json:"resolution,omitempty"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	ImageURLs   []string `json:"image_urls,omitempty"`
}

type submitResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
	Data    []struct {
		Status string `json:"status"`
		TaskID string `json:"task_id"`
	} `json:"data"`
}

type taskStatusResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message,omitempty"`
	Data    taskDataItem `json:"data"`
}

type taskDataItem struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Result   struct {
		Videos []resultAsset `json:"videos"`
		Images []resultAsset `json:"images"`
	} `json:"result"`
	Error *struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type resultAsset struct {
	URL       json.RawMessage `json:"url"`
	ExpiresAt int64           `json:"expires_at,omitempty"`
}

// Init 保存 Global Gateway 渠道运行时上下文。
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction 复用本地基础视频任务校验，保证模型、提示词和图片输入先进入统一上下文。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL 把本地视频提交转到 Global Gateway 的供应商视频生成入口。
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/videos/generations", a.baseURL), nil
}

// BuildRequestHeader 设置 Global Gateway Bearer Token 鉴权和 JSON 请求头。
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody 将本地任务字段转换为 Global Gateway 视频生成请求体。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body := videoGenerationRequest{
		Model:       info.UpstreamModelName,
		Prompt:      req.Prompt,
		Duration:    resolveDuration(req),
		Resolution:  resolveResolution(req.Size),
		AspectRatio: resolveAspectRatio(req.Size),
		ImageURLs:   collectImageURLs(req),
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &body); err != nil {
		return nil, fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	if body.Model == "" {
		body.Model = info.OriginModelName
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest 复用统一任务请求发送逻辑，保留代理、超时和请求体处理能力。
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 解析 Global Gateway 提交响应，并向下游返回 new-api 既有视频任务格式。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var parsed submitResponse
	if err := common.Unmarshal(responseBody, &parsed); err != nil {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("unmarshal global gateway submit response failed: %w, body: %s", err, responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if parsed.Code != 0 && parsed.Code != http.StatusOK {
		message := parsed.Message
		if message == "" {
			message = fmt.Sprintf("global gateway submit failed, code: %d", parsed.Code)
		}
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("%s", message), "global_gateway_submit_failed", http.StatusBadRequest)
		return
	}
	if len(parsed.Data) == 0 || strings.TrimSpace(parsed.Data[0].TaskID) == "" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("global gateway submit response missing task_id"), "invalid_response", http.StatusInternalServerError)
		return
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = info.PublicTaskID
	openAIVideo.TaskID = info.PublicTaskID
	openAIVideo.CreatedAt = time.Now().Unix()
	openAIVideo.Model = info.OriginModelName
	c.JSON(http.StatusOK, openAIVideo)

	return parsed.Data[0].TaskID, responseBody, nil
}

// FetchTask 查询 Global Gateway 通用任务状态接口，用于本地轮询更新任务状态。
func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/tasks/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(taskID))
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// ParseTaskResult 把 Global Gateway 任务状态映射为本地 TaskStatus，并提取视频结果地址。
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var parsed taskStatusResponse
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal global gateway task response failed: %w", err)
	}
	if parsed.Code != 0 && parsed.Code != http.StatusOK {
		reason := parsed.Message
		if reason == "" {
			reason = fmt.Sprintf("global gateway task query failed, code: %d", parsed.Code)
		}
		return &relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: reason, Progress: taskcommon.ProgressComplete}, nil
	}

	taskInfo := &relaycommon.TaskInfo{TaskID: parsed.Data.ID}
	switch parsed.Data.Status {
	case "pending", "queued":
		taskInfo.Status = model.TaskStatusQueued
	case "submitted":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing", "in_progress":
		taskInfo.Status = model.TaskStatusInProgress
	case "completed", "success", "succeeded":
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Url = firstResultURL(parsed.Data.Result.Videos)
		if taskInfo.Url == "" {
			taskInfo.Url = firstResultURL(parsed.Data.Result.Images)
		}
	case "failed", "cancelled", "canceled":
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Reason = providerTaskErrorMessage(parsed.Data)
	default:
		return nil, fmt.Errorf("unknown global gateway task status: %s", parsed.Data.Status)
	}
	if parsed.Data.Progress > 0 {
		taskInfo.Progress = fmt.Sprintf("%d%%", parsed.Data.Progress)
	}
	if taskInfo.Status == model.TaskStatusSuccess || taskInfo.Status == model.TaskStatusFailure {
		taskInfo.Progress = taskcommon.ProgressComplete
	}
	return taskInfo, nil
}

// GetModelList 返回 Global Gateway 常用视频模型，实际可通过模型映射扩展。
func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// EstimateBilling 从请求中提取视频时长（秒），作为 OtherRatios 返回。
// 【修改说明】新增按秒计费支持：当模型配置为 per_second 计费模式时，
// relay_task.go 会将此 seconds 倍率乘到基础额度上，实现按秒计费。
// seconds 来源优先级：req.Duration > req.Seconds > 默认值 4。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	seconds := resolveDuration(req)
	if seconds <= 0 {
		seconds = 4
	}
	return map[string]float64{"seconds": float64(seconds)}
}

// GetChannelName 返回渠道名称，用于模型列表和后台展示。
func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ConvertToOpenAIVideo 将本地任务转换为现有 OpenAI Video 查询响应，保持对外协议稳定。
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	openAIVideo := originTask.ToOpenAIVideo()
	if originTask.Status == model.TaskStatusFailure {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: originTask.FailReason,
			Code:    "task_failed",
		}
	}
	if resultURL := originTask.GetResultURL(); resultURL != "" {
		openAIVideo.SetMetadata("url", resultURL)
	}
	return common.Marshal(openAIVideo)
}

func resolveDuration(req relaycommon.TaskSubmitReq) int {
	if req.Duration > 0 {
		return req.Duration
	}
	seconds, _ := strconv.Atoi(req.Seconds)
	return seconds
}

func collectImageURLs(req relaycommon.TaskSubmitReq) []string {
	urls := append([]string{}, req.Images...)
	if strings.TrimSpace(req.Image) != "" {
		urls = append(urls, strings.TrimSpace(req.Image))
	}
	if strings.TrimSpace(req.InputReference) != "" {
		urls = append(urls, strings.TrimSpace(req.InputReference))
	}
	return urls
}

func resolveResolution(size string) string {
	size = strings.ToLower(strings.TrimSpace(size))
	switch {
	case size == "720p" || strings.Contains(size, "720"):
		return "720p"
	case size == "1024p" || strings.Contains(size, "1024"):
		return "1024p"
	case size == "1080p" || strings.Contains(size, "1080"):
		return "1080p"
	default:
		return ""
	}
}

func resolveAspectRatio(size string) string {
	size = strings.ToLower(strings.TrimSpace(size))
	switch size {
	case "16:9", "landscape":
		return "16:9"
	case "9:16", "portrait":
		return "9:16"
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return ""
	}
	width, errW := strconv.Atoi(parts[0])
	height, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil || width == 0 || height == 0 {
		return ""
	}
	if width >= height {
		return "16:9"
	}
	return "9:16"
}

func firstResultURL(assets []resultAsset) string {
	for _, asset := range assets {
		if url := firstURL(asset.URL); url != "" {
			return url
		}
	}
	return ""
}

func firstURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var urls []string
	if err := common.Unmarshal(raw, &urls); err == nil && len(urls) > 0 {
		return urls[0]
	}
	var url string
	if err := common.Unmarshal(raw, &url); err == nil {
		return url
	}
	return ""
}

func providerTaskErrorMessage(data taskDataItem) string {
	if data.Error == nil {
		return "task failed"
	}
	if data.Error.Message != "" {
		return data.Error.Message
	}
	if data.Error.Code != nil {
		return fmt.Sprintf("task failed: %v", data.Error.Code)
	}
	return "task failed"
}
