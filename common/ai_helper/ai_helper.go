package aihelper

import (
	"NexusAi/common/rabbitmq"
	"NexusAi/config"
	"NexusAi/model"
	mylogger "NexusAi/pkg/logger"
	"NexusAi/pkg/utils"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// 系统提示词模板（modelName 作为参数传入）
func buildSystemPrompt(defaultPrompt, currentDate, modelName string) string {
	return fmt.Sprintf(`%s

【当前时间】今天是 %s。

【当前模型】你现在正在使用 %s 模型提供服务。当用户询问你是什么模型时，请如实告知你正在使用的模型名称。

【重要工具使用规则】
当用户的请求包含以下关键词时，必须调用相应的工具：
- 朗读、播放、念、读、听、语音、朗读给我听、念给我听 → 必须使用 text_to_speech 工具
- 搜索、查、最新、新闻 → 使用 search 工具
- 翻译、translate → 使用 translate 工具
- 天气、weather → 使用 weather 工具

【朗读功能特别说明】
当用户要求朗读时，你必须：
1. 先生成正常的文本回复
2. 然后调用 text_to_speech 工具，将回复内容转换为语音
3. 工具会返回音频链接，用户可以点击播放

请严格按照以上规则执行，不要忽略用户的朗读请求。`, defaultPrompt, currentDate, modelName)
}

type AIHelper struct {
	model           AIModel
	messages        []*model.Message
	mutex           sync.RWMutex
	saveFuncMu      sync.RWMutex      // 保护 saveFunc 的锁
	SessionID       string
	saveFunc        func(*model.Message) error // 消息回调函数，默认存储到 rabbitmq
	pendingMsgID    string                      // 待确认的消息 ID，用于回滚
	agentService    *AgentService               // Agent 服务（可选）
	agentMode       AgentMode                   // Agent 模式
	config          *AIContextConfig            // 上下文配置
	tokenStats      *TokenStats                 // Token 统计
	requestQueue    chan *chatRequest           // 请求队列
	queueRunning    bool                        // 队列是否在运行
	queueMutex      sync.Mutex                  // 保护队列运行状态
}

// chatRequest 聊天请求
type chatRequest struct {
	ctx        context.Context
	userID     string
	question   string
	cb         StreamCallback
	responseCh chan *chatResponse
}

// chatResponse 聊天响应
type chatResponse struct {
	content string
	err     error
}

// TokenStats Token 统计信息
type TokenStats struct {
	TotalInputTokens  int64 // 总输入 Token 数
	TotalOutputTokens int64 // 总输出 Token 数
	TotalTokens       int64 // 总 Token 数
	RequestCount      int64 // 请求次数
	mutex             sync.RWMutex
}

// NewTokenStats 创建 Token 统计实例
func NewTokenStats() *TokenStats {
	return &TokenStats{}
}

// AddInputTokens 添加输入 Token 数
func (t *TokenStats) AddInputTokens(count int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.TotalInputTokens += int64(count)
	t.TotalTokens += int64(count)
	t.RequestCount++
}

// AddOutputTokens 添加输出 Token 数
func (t *TokenStats) AddOutputTokens(count int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.TotalOutputTokens += int64(count)
	t.TotalTokens += int64(count)
}

// GetStats 获取统计数据
func (t *TokenStats) GetStats() (input, output, total int64, requests int64) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.TotalInputTokens, t.TotalOutputTokens, t.TotalTokens, t.RequestCount
}

// AIContextConfig AI 上下文配置
type AIContextConfig struct {
	MaxMessages  int    // 最大消息轮次
	MaxTokens    int    // 最大 Token 数
	Strategy     string // 上下文策略
}

// DefaultAIContextConfig 默认上下文配置
func DefaultAIContextConfig() *AIContextConfig {
	cfg := config.GetConfig().AIConfig
	return &AIContextConfig{
		MaxMessages:  cfg.MaxContextMessages,
		MaxTokens:    cfg.MaxContextTokens,
		Strategy:     cfg.ContextStrategy,
	}
}

// NewAIHelper 创建 AIHelper 实例
func NewAIHelper(aiModel AIModel, sessionID string) *AIHelper {
	h := &AIHelper{
		model:        aiModel,
		messages:     make([]*model.Message, 0),
		saveFunc: func(msg *model.Message) error {
			data, err := rabbitmq.GenerateMessageMQParam(msg.SessionID, msg.Content, msg.UserID, msg.IsUser)
			if err != nil {
				return err
			}
			return rabbitmq.RMQMessage.Publish(data)
		},
		SessionID:    sessionID,
		agentMode:    AgentModeNone,
		config:       DefaultAIContextConfig(),
		tokenStats:   NewTokenStats(),
		requestQueue: make(chan *chatRequest, 100), // 请求队列，最多缓存 100 个请求
	}
	return h
}

// NewAIHelperWithAgent 创建带 Agent 的 AIHelper 实例
func NewAIHelperWithAgent(agentService *AgentService, sessionID string, mode AgentMode) *AIHelper {
	return &AIHelper{
		messages:     make([]*model.Message, 0),
		saveFunc: func(msg *model.Message) error {
			data, err := rabbitmq.GenerateMessageMQParam(msg.SessionID, msg.Content, msg.UserID, msg.IsUser)
			if err != nil {
				return err
			}
			return rabbitmq.RMQMessage.Publish(data)
		},
		SessionID:    sessionID,
		agentService: agentService,
		agentMode:    mode,
		config:       DefaultAIContextConfig(),
		tokenStats:   NewTokenStats(),
		requestQueue: make(chan *chatRequest, 100),
	}
}

// SetAgentMode 设置 Agent 模式
func (a *AIHelper) SetAgentMode(mode AgentMode) {
	a.agentMode = mode
}

// GetAgentMode 获取 Agent 模式
func (a *AIHelper) GetAgentMode() AgentMode {
	return a.agentMode
}

// SetAgentService 设置 Agent 服务
func (a *AIHelper) SetAgentService(agentService *AgentService) {
	a.agentService = agentService
}

// AddMessage 添加消息到对话历史，并通过回调函数保存
func (a *AIHelper) AddMessage(content, userID string, isUser bool, save bool) error {
	msg := &model.Message{
		SessionID: a.SessionID,
		Content:   content,
		UserID:    userID,
		IsUser:    isUser,
	}

	a.mutex.Lock()
	a.messages = append(a.messages, msg)
	a.mutex.Unlock()

	if save {
		if err := a.getSaveFunc()(msg); err != nil {
			mylogger.Logger.Error("save message failed: " + err.Error())
			return err
		}
	}
	return nil
}

// SetSaveFunc 设置消息保存回调函数
func (a *AIHelper) SetSaveFunc(saveFunc func(*model.Message) error) {
	a.saveFuncMu.Lock()
	defer a.saveFuncMu.Unlock()
	a.saveFunc = saveFunc
}

// getSaveFunc 获取消息保存回调函数（线程安全）
func (a *AIHelper) getSaveFunc() func(*model.Message) error {
	a.saveFuncMu.RLock()
	defer a.saveFuncMu.RUnlock()
	return a.saveFunc
}

// GetMessages 获取当前对话历史的消息列表
func (a *AIHelper) GetMessages() []*model.Message {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	out := make([]*model.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

// GetMessageCount 获取消息数量
func (a *AIHelper) GetMessageCount() int {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return len(a.messages)
}

// RemoveLastMessage 移除最后一条消息（用于回滚）
func (a *AIHelper) RemoveLastMessage() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if len(a.messages) > 0 {
		a.messages = a.messages[:len(a.messages)-1]
	}
}

// ========== 上下文管理 ==========

// trimMessagesByCount 根据消息轮次裁剪上下文（滑动窗口策略）
func (a *AIHelper) trimMessagesByCount() {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.config == nil || a.config.MaxMessages <= 0 {
		return
	}

	// 计算最大消息数量（一问一答为1轮，所以消息数是轮次的2倍）
	maxCount := a.config.MaxMessages * 2

	if len(a.messages) > maxCount {
		// 保留最近的消息
		a.messages = a.messages[len(a.messages)-maxCount:]
		mylogger.Logger.Info("context trimmed by sliding window",
			zap.Int("before", len(a.messages)+maxCount),
			zap.Int("after", len(a.messages)),
			zap.Int("maxRounds", a.config.MaxMessages),
		)
	}
}

// estimateTokenCount 粗略估算消息的 Token 数量
// 使用简单的启发式方法：平均 4 个字符约等于 1 个 Token（中文）
func (a *AIHelper) estimateTokenCount(messages []*schema.Message) int {
	totalTokens := 0
	for _, msg := range messages {
		// 粗略估算：中文约 1.5 字符/Token，英文约 4 字符/Token
		// 取折中值：3 字符/Token
		totalTokens += len(msg.Content) / 3
	}
	return totalTokens
}

// trimMessagesByTokens 根据 Token 数量裁剪上下文
func (a *AIHelper) trimMessagesByTokens(messages []*schema.Message) []*schema.Message {
	if a.config == nil || a.config.MaxTokens <= 0 {
		return messages
	}

	totalTokens := a.estimateTokenCount(messages)
	if totalTokens <= a.config.MaxTokens {
		return messages
	}

	// 从头部开始移除消息，直到 Token 数量符合限制
	trimmed := make([]*schema.Message, len(messages))
	copy(trimmed, messages)

	for len(trimmed) > 0 {
		firstMsgTokens := len(trimmed[0].Content) / 3
		if totalTokens-firstMsgTokens <= a.config.MaxTokens {
			break
		}
		totalTokens -= firstMsgTokens
		trimmed = trimmed[1:]
	}

	mylogger.Logger.Info("context trimmed by token limit",
		zap.Int("before", len(messages)),
		zap.Int("after", len(trimmed)),
		zap.Int("tokens", totalTokens),
		zap.Int("maxTokens", a.config.MaxTokens),
	)

	return trimmed
}

// applyContextStrategy 应用上下文策略
func (a *AIHelper) applyContextStrategy(messages []*schema.Message) []*schema.Message {
	if a.config == nil {
		return messages
	}

	switch a.config.Strategy {
	case "sliding_window":
		// 滑动窗口策略：在内存中已经通过 trimMessagesByCount 处理
		// 这里再根据 Token 数量做二次裁剪
		return a.trimMessagesByTokens(messages)
	case "summary":
		// 摘要压缩策略（TODO：需要额外的 LLM 调用）
		// 目前降级为滑动窗口
		return a.trimMessagesByTokens(messages)
	default:
		return a.trimMessagesByTokens(messages)
	}
}

// ========== 系统提示词 ==========

// addSystemPrompt 添加系统提示词（包含当前日期和模型名称）
func (a *AIHelper) addSystemPrompt(messages []*schema.Message) []*schema.Message {
	// 获取当前日期和星期
	now := time.Now()
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	weekday := weekdays[now.Weekday()]
	currentDate := fmt.Sprintf("%d年%d月%d日（%s）", now.Year(), now.Month(), now.Day(), weekday)

	// 获取用户自定义的系统提示词（如果有的话）
	defaultPrompt := os.Getenv("DEFAULT_SYSTEM_PROMPT")
	if defaultPrompt == "" {
		defaultPrompt = "你是一个智能助手，帮助用户解决问题。"
	}

	// 获取当前模型名称
	modelName := a.getModelName()

	// 构建系统提示词
	systemPrompt := buildSystemPrompt(defaultPrompt, currentDate, modelName)

	// 创建系统消息
	systemMsg := &schema.Message{
		Role:    schema.System,
		Content: systemPrompt,
	}

	// 将系统消息插入到消息列表最前面
	result := make([]*schema.Message, 0, len(messages)+1)
	result = append(result, systemMsg)
	result = append(result, messages...)

	return result
}

// getModelName 获取当前模型名称
func (a *AIHelper) getModelName() string {
	if a.agentService != nil {
		return a.agentService.GetModelName()
	}
	if a.model != nil {
		return a.model.GetModelName()
	}
	return "未知模型"
}

// GenerateResponse 生成 AI 响应，并将用户问题和 AI 回答添加到对话历史中
func (a *AIHelper) GenerateResponse(ctx context.Context, userID string, userQuestion string) (*model.Message, error) {
	// 先添加到内存，不保存到外部存储
	if err := a.AddMessage(userQuestion, userID, true, false); err != nil {
		return nil, err
	}

	// 应用滑动窗口裁剪上下文
	a.trimMessagesByCount()

	a.mutex.RLock()
	messages := utils.ConvertToSchemaMessages(a.messages)
	a.mutex.RUnlock()

	// 应用上下文策略（Token 裁剪等）
	messages = a.applyContextStrategy(messages)

	// 添加系统提示词（包含当前日期）
	messages = a.addSystemPrompt(messages)

	var schemaMsg *schema.Message
	var err error

	// 根据 Agent 模式选择不同的响应方式
	if a.agentMode == AgentModeReAct && a.agentService != nil {
		schemaMsg, err = a.agentService.GenerateResponse(ctx, messages)
	} else if a.model != nil {
		schemaMsg, err = a.model.GenerateResponse(ctx, messages)
	} else {
		return nil, fmt.Errorf("no model or agent service configured")
	}

	if err != nil {
		// AI 调用失败，回滚内存中的用户消息
		a.RemoveLastMessage()
		return nil, err
	}

	// AI 调用成功，先保存用户消息
	userMsg := &model.Message{
		SessionID: a.SessionID,
		Content:   userQuestion,
		UserID:    userID,
		IsUser:    true,
	}
	if err := a.getSaveFunc()(userMsg); err != nil {
		mylogger.Logger.Error("save user message failed: " + err.Error())
		// 保存失败不影响流程，继续保存 AI 响应
	}

	// 将 AI 回答转换为数据库消息格式，并添加到对话历史中
	modelMsg := utils.ConvertToModelMessage(a.SessionID, userID, schemaMsg)
	if err := a.AddMessage(modelMsg.Content, userID, false, true); err != nil {
		return nil, err
	}

	// 统计 Token（估算）
	if a.tokenStats != nil {
		inputTokens := a.estimateTokenCount(messages)
		outputTokens := len(schemaMsg.Content) / 3
		a.tokenStats.AddInputTokens(inputTokens)
		a.tokenStats.AddOutputTokens(outputTokens)
		mylogger.Logger.Info("token stats",
			zap.Int("input", inputTokens),
			zap.Int("output", outputTokens),
		)
	}

	return modelMsg, nil
}

// StreamResponse 实现流式响应，实时回调消息片段，并将完整响应添加到对话历史中
func (a *AIHelper) StreamResponse(ctx context.Context, userID string, userQuestion string, cb StreamCallback) (string, error) {
	// 先添加到内存，不保存到外部存储
	if err := a.AddMessage(userQuestion, userID, true, false); err != nil {
		return "", err
	}

	// 应用滑动窗口裁剪上下文
	a.trimMessagesByCount()

	a.mutex.RLock()
	messages := utils.ConvertToSchemaMessages(a.messages)
	a.mutex.RUnlock()

	// 应用上下文策略（Token 裁剪等）
	messages = a.applyContextStrategy(messages)

	// 添加系统提示词（包含当前日期）
	messages = a.addSystemPrompt(messages)

	var content string
	var err error

	// 根据 Agent 模式选择不同的响应方式
	if a.agentMode == AgentModeReAct && a.agentService != nil {
		content, err = a.agentService.StreamResponse(ctx, messages, cb)
	} else if a.model != nil {
		content, err = a.model.StreamResponse(ctx, messages, cb)
	} else {
		return "", fmt.Errorf("no model or agent service configured")
	}

	if err != nil {
		// 流式调用失败，回滚内存中的用户消息
		a.RemoveLastMessage()
		return "", err
	}

	// AI 调用成功，先保存用户消息
	userMsg := &model.Message{
		SessionID: a.SessionID,
		Content:   userQuestion,
		UserID:    userID,
		IsUser:    true,
	}
	if err := a.getSaveFunc()(userMsg); err != nil {
		mylogger.Logger.Error("save user message failed: " + err.Error())
		// 保存失败不影响流程，继续保存 AI 响应
	}

	if err := a.AddMessage(content, userID, false, true); err != nil {
		return "", err
	}

	// 统计 Token（估算）
	if a.tokenStats != nil {
		inputTokens := a.estimateTokenCount(messages)
		outputTokens := len(content) / 3
		a.tokenStats.AddInputTokens(inputTokens)
		a.tokenStats.AddOutputTokens(outputTokens)
		mylogger.Logger.Info("token stats",
			zap.Int("input", inputTokens),
			zap.Int("output", outputTokens),
		)
	}

	return content, nil
}

// ========== 会话级请求队列（防止并发消息错乱） ==========

// QueueGenerateResponse 非流式请求队列
func (a *AIHelper) QueueGenerateResponse(ctx context.Context, userID, question string) (*model.Message, error) {
	// 创建请求（cb 为空，表示非流式）
	req := &chatRequest{
		ctx:        ctx,
		userID:     userID,
		question:   question,
		cb:         nil, // 非流式
		responseCh: make(chan *chatResponse, 1),
	}

	// 将请求加入队列
	select {
	case a.requestQueue <- req:
		// 成功加入队列
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 启动队列处理（如果尚未启动）
	go a.processQueue()

	// 等待响应
	select {
	case resp := <-req.responseCh:
		if resp.err != nil {
			return nil, resp.err
		}
		// 返回消息结构
		return &model.Message{
			SessionID: a.SessionID,
			Content:   resp.content,
			UserID:    userID,
			IsUser:    false,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// QueueStreamResponse 将请求加入队列并等待响应
// 确保同一会话内的消息按顺序处理
func (a *AIHelper) QueueStreamResponse(ctx context.Context, userID, question string, cb StreamCallback) (string, error) {
	// 创建请求
	req := &chatRequest{
		ctx:        ctx,
		userID:     userID,
		question:   question,
		cb:         cb,
		responseCh: make(chan *chatResponse, 1),
	}

	// 将请求加入队列
	select {
	case a.requestQueue <- req:
		// 成功加入队列
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// 启动队列处理（如果尚未启动）
	go a.processQueue()

	// 等待响应
	select {
	case resp := <-req.responseCh:
		return resp.content, resp.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// processQueue 处理请求队列
func (a *AIHelper) processQueue() {
	a.queueMutex.Lock()
	if a.queueRunning {
		a.queueMutex.Unlock()
		return
	}
	a.queueRunning = true
	a.queueMutex.Unlock()

	defer func() {
		a.queueMutex.Lock()
		a.queueRunning = false
		a.queueMutex.Unlock()
	}()

	for {
		select {
		case req := <-a.requestQueue:
			// 处理请求
			if req.cb != nil {
				// 流式响应
				content, err := a.streamResponseInternal(req.ctx, req.userID, req.question, req.cb)
				req.responseCh <- &chatResponse{content: content, err: err}
			} else {
				// 非流式响应
				msg, err := a.generateResponseInternal(req.ctx, req.userID, req.question)
				if err != nil {
					req.responseCh <- &chatResponse{content: "", err: err}
				} else {
					req.responseCh <- &chatResponse{content: msg.Content, err: nil}
				}
			}
		default:
			// 队列为空，退出
			return
		}
	}
}

// generateResponseInternal 内部非流式响应方法
func (a *AIHelper) generateResponseInternal(ctx context.Context, userID string, userQuestion string) (*model.Message, error) {
	// 先添加到内存，不保存到外部存储
	if err := a.AddMessage(userQuestion, userID, true, false); err != nil {
		return nil, err
	}

	// 应用滑动窗口裁剪上下文
	a.trimMessagesByCount()

	a.mutex.RLock()
	messages := utils.ConvertToSchemaMessages(a.messages)
	a.mutex.RUnlock()

	// 应用上下文策略（Token 裁剪等）
	messages = a.applyContextStrategy(messages)

	// 添加系统提示词（包含当前日期）
	messages = a.addSystemPrompt(messages)

	var schemaMsg *schema.Message
	var err error

	// 根据 Agent 模式选择不同的响应方式
	if a.agentMode == AgentModeReAct && a.agentService != nil {
		schemaMsg, err = a.agentService.GenerateResponse(ctx, messages)
	} else if a.model != nil {
		schemaMsg, err = a.model.GenerateResponse(ctx, messages)
	} else {
		return nil, fmt.Errorf("no model or agent service configured")
	}

	if err != nil {
		// AI 调用失败，回滚内存中的用户消息
		a.RemoveLastMessage()
		return nil, err
	}

	// AI 调用成功，先保存用户消息
	userMsg := &model.Message{
		SessionID: a.SessionID,
		Content:   userQuestion,
		UserID:    userID,
		IsUser:    true,
	}
	if err := a.getSaveFunc()(userMsg); err != nil {
		mylogger.Logger.Error("save user message failed: " + err.Error())
		// 保存失败不影响流程，继续保存 AI 响应
	}

	// 将 AI 回答转换为数据库消息格式，并添加到对话历史中
	modelMsg := utils.ConvertToModelMessage(a.SessionID, userID, schemaMsg)
	if err := a.AddMessage(modelMsg.Content, userID, false, true); err != nil {
		return nil, err
	}

	// 统计 Token（估算）
	if a.tokenStats != nil {
		inputTokens := a.estimateTokenCount(messages)
		outputTokens := len(schemaMsg.Content) / 3
		a.tokenStats.AddInputTokens(inputTokens)
		a.tokenStats.AddOutputTokens(outputTokens)
		mylogger.Logger.Info("token stats",
			zap.Int("input", inputTokens),
			zap.Int("output", outputTokens),
		)
	}

	return modelMsg, nil
}

// streamResponseInternal 内部流式响应方法
func (a *AIHelper) streamResponseInternal(ctx context.Context, userID string, userQuestion string, cb StreamCallback) (string, error) {
	// 先添加到内存，不保存到外部存储
	if err := a.AddMessage(userQuestion, userID, true, false); err != nil {
		return "", err
	}

	// 应用滑动窗口裁剪上下文
	a.trimMessagesByCount()

	a.mutex.RLock()
	messages := utils.ConvertToSchemaMessages(a.messages)
	a.mutex.RUnlock()

	// 应用上下文策略（Token 裁剪等）
	messages = a.applyContextStrategy(messages)

	// 添加系统提示词（包含当前日期）
	messages = a.addSystemPrompt(messages)

	var content string
	var err error

	// 根据 Agent 模式选择不同的响应方式
	if a.agentMode == AgentModeReAct && a.agentService != nil {
		content, err = a.agentService.StreamResponse(ctx, messages, cb)
	} else if a.model != nil {
		content, err = a.model.StreamResponse(ctx, messages, cb)
	} else {
		return "", fmt.Errorf("no model or agent service configured")
	}

	if err != nil {
		// 流式调用失败，回滚内存中的用户消息
		a.RemoveLastMessage()
		return "", err
	}

	// AI 调用成功，先保存用户消息
	userMsg := &model.Message{
		SessionID: a.SessionID,
		Content:   userQuestion,
		UserID:    userID,
		IsUser:    true,
	}
	if err := a.getSaveFunc()(userMsg); err != nil {
		mylogger.Logger.Error("save user message failed: " + err.Error())
		// 保存失败不影响流程，继续保存 AI 响应
	}

	if err := a.AddMessage(content, userID, false, true); err != nil {
		return "", err
	}

	// 统计 Token（估算）
	if a.tokenStats != nil {
		inputTokens := a.estimateTokenCount(messages)
		outputTokens := len(content) / 3
		a.tokenStats.AddInputTokens(inputTokens)
		a.tokenStats.AddOutputTokens(outputTokens)
		mylogger.Logger.Info("token stats",
			zap.Int("input", inputTokens),
			zap.Int("output", outputTokens),
		)
	}

	return content, nil
}

// GetTokenStats 获取 Token 统计信息
func (a *AIHelper) GetTokenStats() (input, output, total int64, requests int64) {
	if a.tokenStats == nil {
		return 0, 0, 0, 0
	}
	return a.tokenStats.GetStats()
}

// GetModelType 获取当前使用的 AI 模型类型
func (a *AIHelper) GetModelType() string {
	if a.agentService != nil {
		return a.agentService.GetModelType()
	}
	if a.model != nil {
		return a.model.GetModelType()
	}
	return "unknown"
}
